package cluster

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	headerProtocol     = "X-KPanel-Federation-Protocol"
	headerControllerID = "X-KPanel-Controller-ID"
	headerTargetID     = "X-KPanel-Target-ID"
	headerTimestamp    = "X-KPanel-Timestamp"
	headerNonce        = "X-KPanel-Nonce"
	headerSignature    = "X-KPanel-Signature"
)

const (
	FederationCapabilitiesHeader   = "X-KPanel-Response-Capabilities"
	SecurityEntrancePathCapability = "security-entrance-path-v1"
)

// Response capabilities are transport hints, not authorization inputs. V1 is
// HTTPS-only; V2 still encrypts the optional response field inside Noise.
func hasFederationCapability(value string, capability string) bool {
	if len(value) > 256 || capability == "" {
		return false
	}
	for _, item := range strings.Split(value, ",") {
		if strings.TrimSpace(item) == capability {
			return true
		}
	}
	return false
}

func SignRequest(request *http.Request, controllerID, targetID string, privateKey ed25519.PrivateKey, now time.Time, nonce string) error {
	if request == nil || len(privateKey) != ed25519.PrivateKeySize ||
		!validID(controllerID) || !validID(targetID) || !validNonce(nonce) {
		return errors.New("federation signing input is invalid")
	}
	timestamp := strconv.FormatInt(now.UTC().Unix(), 10)
	canonical := canonicalRequest(request.Method, request.URL.EscapedPath(), controllerID, targetID, timestamp, nonce)
	signature := ed25519.Sign(privateKey, []byte(canonical))
	request.Header.Set(headerProtocol, FederationProtocol)
	request.Header.Set(headerControllerID, controllerID)
	request.Header.Set(headerTargetID, targetID)
	request.Header.Set(headerTimestamp, timestamp)
	request.Header.Set(headerNonce, nonce)
	request.Header.Set(headerSignature, base64.RawStdEncoding.EncodeToString(signature))
	return nil
}

func VerifyRequest(request *http.Request, targetID string, publicKey ed25519.PublicKey, now time.Time) (string, string, error) {
	if request == nil || len(publicKey) != ed25519.PublicKeySize {
		return "", "", ErrAuthentication
	}
	if request.Header.Get(headerProtocol) != FederationProtocol {
		return "", "", ErrProtocolMismatch
	}
	controllerID := strings.TrimSpace(request.Header.Get(headerControllerID))
	receivedTarget := strings.TrimSpace(request.Header.Get(headerTargetID))
	timestamp := strings.TrimSpace(request.Header.Get(headerTimestamp))
	nonce := strings.TrimSpace(request.Header.Get(headerNonce))
	if !validID(controllerID) || receivedTarget != targetID || !validNonce(nonce) {
		return "", "", ErrAuthentication
	}
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || seconds < 1 {
		return "", "", ErrAuthentication
	}
	requestTime := time.Unix(seconds, 0).UTC()
	if delta := now.UTC().Sub(requestTime); delta < -60*time.Second || delta > 60*time.Second {
		return "", "", ErrAuthentication
	}
	encodedSignature := strings.TrimSpace(request.Header.Get(headerSignature))
	if len(encodedSignature) > 128 {
		return "", "", ErrAuthentication
	}
	signature, err := base64.RawStdEncoding.DecodeString(encodedSignature)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return "", "", ErrAuthentication
	}
	canonical := canonicalRequest(request.Method, request.URL.EscapedPath(), controllerID, targetID, timestamp, nonce)
	if !ed25519.Verify(publicKey, []byte(canonical), signature) {
		return "", "", ErrAuthentication
	}
	return controllerID, nonce, nil
}

func canonicalRequest(method, path, controllerID, targetID, timestamp, nonce string) string {
	empty := sha256.Sum256(nil)
	return strings.Join([]string{
		strings.ToUpper(method), path, controllerID, targetID, timestamp, nonce,
		hex.EncodeToString(empty[:]),
	}, "\n")
}

func fingerprint(publicKey ed25519.PublicKey) string {
	sum := sha256.Sum256(publicKey)
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:])
}

func decodePublicKey(value string) (ed25519.PublicKey, error) {
	if len(value) > 64 {
		return nil, errors.New("federation public key is too large")
	}
	decoded, err := base64.RawStdEncoding.DecodeString(value)
	if err != nil || len(decoded) != ed25519.PublicKeySize {
		return nil, errors.New("federation public key is invalid")
	}
	return ed25519.PublicKey(append([]byte(nil), decoded...)), nil
}

func encodePublicKey(value ed25519.PublicKey) string {
	return base64.RawStdEncoding.EncodeToString(value)
}

func validNonce(value string) bool {
	if len(value) != 32 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 16
}

func signatureHeaders() []string {
	return []string{
		headerProtocol, headerControllerID, headerTargetID,
		headerTimestamp, headerNonce, headerSignature,
	}
}

func requireSignedGET(request *http.Request) error {
	if request.Method != http.MethodGet || request.URL.RawQuery != "" ||
		request.URL.RawPath != "" || request.ContentLength != 0 ||
		len(request.TransferEncoding) != 0 {
		return fmt.Errorf("federation summary request shape is invalid")
	}
	return nil
}
