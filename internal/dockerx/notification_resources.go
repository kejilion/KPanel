package dockerx

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
)

const MaxNotificationContainers = 128

var ErrNotificationResourceLimit = errors.New("notification container limit reached")

// NotificationContainer deliberately excludes inspect environment, labels,
// mounts, networking and credentials. No Docker mutation is available here.
type NotificationContainer struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	State           string `json:"state"`
	Health          string `json:"health,omitempty"`
	RestartCount    *int64 `json:"restartCount,omitempty"`
	ResourceVersion string `json:"resourceVersion"`
	Known           bool   `json:"known"`
}

func (c *Client) NotificationResources(ctx context.Context) ([]NotificationContainer, error) {
	var list []struct {
		ID    string   `json:"Id"`
		Names []string `json:"Names"`
	}
	if err := c.notificationJSON(ctx, "/containers/json?all=1&size=0", &list); err != nil {
		return nil, err
	}
	if len(list) > MaxNotificationContainers {
		return nil, ErrNotificationResourceLimit
	}
	result := make([]NotificationContainer, len(list))
	indices := make(chan int)
	var wg sync.WaitGroup
	for range min(4, len(list)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range indices {
				item := list[i]
				entry := NotificationContainer{ID: item.ID, Name: item.ID}
				if len(item.Names) > 0 {
					entry.Name = strings.TrimPrefix(item.Names[0], "/")
				}
				if len(item.ID) != 64 || !containerIDPattern.MatchString(item.ID) {
					result[i] = entry
					continue
				}
				var raw struct {
					ID           string `json:"Id"`
					Name         string `json:"Name"`
					RestartCount *int64 `json:"RestartCount"`
					State        *struct {
						Status string `json:"Status"`
						Health *struct {
							Status string `json:"Status"`
						} `json:"Health"`
					} `json:"State"`
				}
				if err := c.notificationJSON(ctx, "/containers/"+item.ID+"/json", &raw); err == nil && raw.ID == item.ID && raw.State != nil && raw.RestartCount != nil && *raw.RestartCount >= 0 {
					entry.Name = strings.TrimPrefix(raw.Name, "/")
					entry.State = raw.State.Status
					entry.RestartCount = raw.RestartCount
					if raw.State.Health != nil {
						entry.Health = raw.State.Health.Status
					}
					entry.Known = true
					entry.ResourceVersion = resourceHash(raw)
				}
				result[i] = entry
			}
		}()
	}
	for i := range list {
		select {
		case indices <- i:
		case <-ctx.Done():
			close(indices)
			wg.Wait()
			return nil, ctx.Err()
		}
	}
	close(indices)
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func (c *Client) notificationJSON(ctx context.Context, path string, target any) error {
	data, truncated, err := c.getBytes(ctx, path, 256<<10)
	if err != nil {
		return err
	}
	if truncated {
		return ErrNotificationResourceLimit
	}
	return json.Unmarshal(data, target)
}
