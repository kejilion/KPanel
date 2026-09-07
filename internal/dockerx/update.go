package dockerx

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var (
	digestPattern        = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
	dockerHubPathPattern = regexp.MustCompile(
		`^[a-z0-9]+(?:[._-][a-z0-9]+)*(?:/[a-z0-9]+(?:[._-][a-z0-9]+)*)*$`,
	)
	dockerTagPattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`)
)

const (
	ImageUpdateCheckTimeout    = 20 * time.Second
	officialUpdateCheckTimeout = 4 * time.Second
	countryLookupTimeout       = 250 * time.Millisecond
	acceleratorCheckTimeout    = 4 * time.Second
)

var dockerHubUpdateAccelerators = []string{
	"docker.1ms.run",
	"gh.kejilion.pro",
}

// All consumers share a small, non-queuing read budget. No check pulls an image.
var imageUpdateSlots = make(chan struct{}, 2)

var ErrImageUpdateFixed = errors.New("image reference is fixed to an immutable digest")
var ErrImageUpdateBusy = errors.New("image update checks are busy")
var ErrImageUpdateDigestMissing = errors.New("running image does not expose a digest for this repository")
var ErrImageUpdateIncomparable = errors.New("image digest identity cannot be compared reliably")

type updateDescriptor struct {
	Digest    string `json:"digest"`
	MediaType string `json:"mediaType"`
}

type updateDistribution struct {
	Descriptor updateDescriptor `json:"Descriptor"`
	Platforms  []struct {
		OS           string `json:"os"`
		Architecture string `json:"architecture"`
		Variant      string `json:"variant"`
	} `json:"Platforms"`
	Reference string `json:"-"`
}

type ImageUpdateResult struct {
	ContainerID     string    `json:"containerId"`
	Image           string    `json:"image"`
	Status          string    `json:"status"`
	UpdateAvailable bool      `json:"updateAvailable"`
	LocalDigest     string    `json:"localDigest,omitempty"`
	RemoteDigest    string    `json:"remoteDigest,omitempty"`
	ResourceVersion string    `json:"resourceVersion"`
	CheckedAt       time.Time `json:"checkedAt"`
}

func (c *Client) CheckContainerImageUpdate(
	ctx context.Context,
	id string,
	expectedVersion string,
) (ImageUpdateResult, error) {
	if !containerIDPattern.MatchString(id) {
		return ImageUpdateResult{}, errors.New("invalid container id")
	}
	if expectedVersion == "" {
		return ImageUpdateResult{}, ErrVersionRequired
	}
	select {
	case imageUpdateSlots <- struct{}{}:
		defer func() { <-imageUpdateSlots }()
	default:
		return ImageUpdateResult{}, ErrImageUpdateBusy
	}
	ctx, cancel := context.WithTimeout(ctx, ImageUpdateCheckTimeout)
	defer cancel()
	// This read-only check uses a fresh snapshot, as the application market
	// does. Mutating operations still require an exact browser resource version.
	// A racing snapshot gets one retry within the same deadline and read slot.
	for attempt := 0; ; attempt++ {
		result, err := c.checkContainerImageSnapshot(ctx, id)
		if attempt == 1 || !errors.Is(err, ErrResourceConflict) || ctx.Err() != nil {
			return result, err
		}
	}
}

func (c *Client) checkContainerImageSnapshot(ctx context.Context, id string) (ImageUpdateResult, error) {
	raw, err := c.inspect(ctx, id)
	if err != nil {
		return ImageUpdateResult{}, err
	}
	summary := c.summaryFromInspect(raw)
	image := strings.TrimSpace(raw.Config.Image)
	localImage := strings.TrimSpace(raw.Image)
	if strings.HasPrefix(image, "sha256:") &&
		raw.Config.Labels["io.kejilion.panel.managed"] == "true" {
		image = strings.TrimSpace(raw.Config.Labels["io.kejilion.panel.image"])
	}
	result := ImageUpdateResult{ContainerID: raw.ID, Image: image,
		ResourceVersion: summary.ResourceVersion, CheckedAt: c.now().UTC()}
	if strings.Contains(image, "@sha256:") || digestPattern.MatchString(image) {
		result.Status = "fixed"
		// Preserve the sentinel for Go consumers; both HTTP entry points expose fixed.
		return result, errors.Join(ErrActionUnsupported, ErrImageUpdateFixed)
	}
	repository, valid := normalizedImageRepository(image)
	if !valid || !digestPattern.MatchString(localImage) {
		return ImageUpdateResult{}, ErrActionUnsupported
	}
	var local struct {
		ID          string           `json:"Id"`
		RepoDigests []string         `json:"RepoDigests"`
		Descriptor  updateDescriptor `json:"Descriptor"`
	}
	if err := c.getJSON(ctx, "/images/"+url.PathEscape(localImage)+"/json", &local); err != nil {
		return ImageUpdateResult{}, err
	}
	localDigests := make(map[string]bool)
	for _, value := range local.RepoDigests {
		name, digest, found := strings.Cut(value, "@")
		normalized, ok := normalizedImageRepository(name)
		if found && ok && normalized == repository && digestPattern.MatchString(digest) {
			localDigests[digest] = true
		}
	}
	localKindHint := ""
	// The containerd image store drops repository aliases when a tag moves,
	// but keeps the content descriptor of the image inspected by immutable ID.
	// Never substitute a descriptor for an explicitly different repository.
	if len(local.RepoDigests) == 0 && local.ID == localImage &&
		digestPattern.MatchString(local.Descriptor.Digest) && updateDigestKind(local.Descriptor.MediaType) != "" {
		localDigests[local.Descriptor.Digest] = true
		localKindHint = updateDigestKind(local.Descriptor.MediaType)
	}
	if len(localDigests) == 0 {
		return ImageUpdateResult{}, ErrImageUpdateDigestMissing
	}
	// A matching Engine descriptor already proves the local index kind. Old
	// registry manifests may have been removed; do not require them again.
	knownLocalIndex := local.ID == localImage && localDigests[local.Descriptor.Digest] &&
		updateDigestKind(local.Descriptor.MediaType) == "index"
	remote, err := c.remoteImageDistributionForUpdate(ctx, image)
	if err != nil {
		return ImageUpdateResult{}, err
	}
	remoteDigest := remote.Descriptor.Digest
	localDigest := remoteDigest
	if localDigests[remoteDigest] && localKindHint != "" && localKindHint != updateDigestKind(remote.Descriptor.MediaType) {
		return ImageUpdateResult{}, ErrImageUpdateIncomparable
	}
	if !localDigests[remoteDigest] {
		if len(localDigests) != 1 {
			return ImageUpdateResult{}, ErrImageUpdateIncomparable
		}
		for digest := range localDigests {
			localDigest = digest
		}
		// Engine's distribution API exposes the top-level descriptor, not index children.
		// Resolve the old digest's kind before comparing: index vs platform is unknown.
		remoteRepository, ok := normalizedImageRepository(remote.Reference)
		if !ok {
			return ImageUpdateResult{}, ErrActionUnsupported
		}
		kind := updateDigestKind(remote.Descriptor.MediaType)
		localRemote := updateDistribution{Descriptor: local.Descriptor}
		if !knownLocalIndex {
			localRemote, err = c.distributionForUpdate(ctx, remoteRepository+"@"+localDigest)
			if err != nil {
				return ImageUpdateResult{}, err
			}
		}
		if localRemote.Descriptor.Digest != localDigest || kind == "" || kind != updateDigestKind(localRemote.Descriptor.MediaType) ||
			(localKindHint != "" && localKindHint != kind) {
			return ImageUpdateResult{}, ErrImageUpdateIncomparable
		}
		if kind == "manifest" && (len(remote.Platforms) != 1 || len(localRemote.Platforms) != 1 ||
			remote.Platforms[0].OS == "" || remote.Platforms[0].Architecture == "" || remote.Platforms[0] != localRemote.Platforms[0]) {
			return ImageUpdateResult{}, ErrImageUpdateIncomparable
		}
	}
	refreshed, err := c.inspect(ctx, id)
	if err != nil {
		return ImageUpdateResult{}, err
	}
	if refreshed.Image != raw.Image || c.summaryFromInspect(refreshed).ResourceVersion != summary.ResourceVersion {
		return ImageUpdateResult{}, ErrResourceConflict
	}
	available := localDigest != remoteDigest
	status := "current"
	if available {
		status = "available"
	}
	return ImageUpdateResult{
		ContainerID: raw.ID, Image: image, Status: status, UpdateAvailable: available,
		LocalDigest: localDigest, RemoteDigest: remoteDigest,
		ResourceVersion: summary.ResourceVersion, CheckedAt: c.now().UTC(),
	}, nil
}

func (c *Client) remoteImageDigestForUpdate(ctx context.Context, image string) (string, error) {
	result, err := c.remoteImageDistributionForUpdate(ctx, image)
	return result.Descriptor.Digest, err
}

func (c *Client) remoteImageDistributionForUpdate(ctx context.Context, image string) (updateDistribution, error) {
	repository, tag, dockerHubImage := normalizedDockerHubReference(image)
	if !dockerHubImage || c.imageUpdateCountry == nil {
		return c.distributionForUpdate(ctx, image)
	}

	countryContext, countryCancel := context.WithTimeout(ctx, countryLookupTimeout)
	country, countryErr := c.imageUpdateCountry(countryContext)
	countryCancel()
	country = strings.ToUpper(strings.TrimSpace(country))
	if countryErr == nil && country != "" && country != "CN" && country != "HK" {
		return c.distributionForUpdate(ctx, image)
	}

	officialContext, cancel := context.WithTimeout(ctx, officialUpdateCheckTimeout)
	digest, officialErr := c.distributionForUpdate(officialContext, image)
	cancel()
	if officialErr == nil {
		return digest, nil
	}

	for _, accelerator := range dockerHubUpdateAccelerators {
		fallbackContext, fallbackCancel := context.WithTimeout(ctx, acceleratorCheckTimeout)
		fallbackImage := accelerator + "/" + repository + ":" + tag
		fallbackDigest, err := c.distributionForUpdate(fallbackContext, fallbackImage)
		fallbackCancel()
		if err == nil {
			return fallbackDigest, nil
		}
	}
	return updateDistribution{}, fmt.Errorf("Docker Hub update check failed after accelerated fallback: %w", officialErr)
}

func (c *Client) distributionForUpdate(ctx context.Context, image string) (updateDistribution, error) {
	var remote updateDistribution
	if err := c.getJSON(ctx, "/distribution/"+url.PathEscape(image)+"/json", &remote); err != nil {
		return remote, err
	}
	if !digestPattern.MatchString(remote.Descriptor.Digest) {
		return remote, errors.New("registry returned an invalid image digest")
	}
	remote.Reference = image
	return remote, nil
}

func updateDigestKind(mediaType string) string {
	switch mediaType {
	case "application/vnd.oci.image.index.v1+json", "application/vnd.docker.distribution.manifest.list.v2+json":
		return "index"
	case "application/vnd.oci.image.manifest.v1+json", "application/vnd.docker.distribution.manifest.v2+json":
		return "manifest"
	default:
		return ""
	}
}

func normalizedImageRepository(image string) (string, bool) {
	if repository, _, ok := normalizedDockerHubReference(image); ok {
		return "docker.io/" + repository, true
	}
	value := strings.TrimSpace(image)
	if strings.ContainsAny(value, "@?#\\") {
		return "", false
	}
	parts := strings.SplitN(value, "/", 2)
	if len(parts) != 2 || !isRegistryComponent(parts[0]) {
		return "", false
	}
	repository := parts[1]
	if colon := strings.LastIndexByte(repository, ':'); colon > strings.LastIndexByte(repository, '/') {
		if !dockerTagPattern.MatchString(repository[colon+1:]) {
			return "", false
		}
		repository = repository[:colon]
	}
	if len(repository) > 255 || !dockerHubPathPattern.MatchString(repository) {
		return "", false
	}
	return strings.ToLower(parts[0]) + "/" + repository, true
}

func normalizedDockerHubReference(image string) (string, string, bool) {
	value := strings.TrimSpace(image)
	if value == "" || strings.Contains(value, "@") || strings.ContainsAny(value, "?#\\") {
		return "", "", false
	}
	parts := strings.Split(value, "/")
	if len(parts) > 1 && isRegistryComponent(parts[0]) {
		switch parts[0] {
		case "docker.io", "index.docker.io", "registry-1.docker.io":
			parts = parts[1:]
		default:
			return "", "", false
		}
	}
	if len(parts) == 0 {
		return "", "", false
	}
	last := parts[len(parts)-1]
	tag := "latest"
	if index := strings.LastIndexByte(last, ':'); index >= 0 {
		tag = last[index+1:]
		parts[len(parts)-1] = last[:index]
	}
	if len(parts) == 1 {
		parts = append([]string{"library"}, parts...)
	}
	repository := strings.Join(parts, "/")
	if len(repository) > 255 || !dockerHubPathPattern.MatchString(repository) ||
		!dockerTagPattern.MatchString(tag) {
		return "", "", false
	}
	return repository, tag, true
}

func isRegistryComponent(value string) bool {
	return value == "localhost" || strings.ContainsAny(value, ".:")
}
