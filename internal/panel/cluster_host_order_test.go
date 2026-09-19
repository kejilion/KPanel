package panel

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestClusterHostOrderLifecycleAndInventorySnapshot(t *testing.T) {
	server, tokenPath := newTestServer(t)
	unauthenticated := performRequest(server, http.MethodGet, "/api/v1/cluster/host-order", nil, nil)
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated host order status = %d; body=%s", unauthenticated.Code, unauthenticated.Body.String())
	}

	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	inventoryResponse := authenticatedRequest(
		server, http.MethodGet, "/api/v1/cluster/hosts", nil,
		sessionCookie, csrfCookie, nil,
	)
	if inventoryResponse.Code != http.StatusOK {
		t.Fatalf("cluster hosts status = %d; body=%s", inventoryResponse.Code, inventoryResponse.Body.String())
	}
	var inventory clusterHostsResponse
	if err := json.Unmarshal(inventoryResponse.Body.Bytes(), &inventory); err != nil {
		t.Fatal(err)
	}
	if inventory.HostOrder.Configured || len(inventory.HostOrder.IDs) != 0 || inventory.HostOrder.ResourceVersion == "" {
		t.Fatalf("unexpected initial host order snapshot: %#v", inventory.HostOrder)
	}

	input := clusterHostOrderInput{
		IDs:                     []string{"remote-b", "local", "remote-a"},
		ExpectedResourceVersion: inventory.HostOrder.ResourceVersion,
	}
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	missingOrigin := authenticatedRequest(
		server, http.MethodPut, "/api/v1/cluster/host-order", body,
		sessionCookie, csrfCookie, map[string]string{
			"Content-Type": "application/json",
			"X-CSRF-Token": csrfCookie.Value,
		},
	)
	if missingOrigin.Code != http.StatusForbidden || !strings.Contains(missingOrigin.Body.String(), "origin_validation_failed") {
		t.Fatalf("missing Origin returned %d %s", missingOrigin.Code, missingOrigin.Body.String())
	}
	missingCSRF := authenticatedRequest(
		server, http.MethodPut, "/api/v1/cluster/host-order", body,
		sessionCookie, csrfCookie, map[string]string{
			"Content-Type": "application/json",
			"Origin":       "http://panel.test",
		},
	)
	if missingCSRF.Code != http.StatusForbidden || !strings.Contains(missingCSRF.Body.String(), "csrf_validation_failed") {
		t.Fatalf("missing CSRF returned %d %s", missingCSRF.Code, missingCSRF.Body.String())
	}

	updatedResponse := authenticatedRequest(
		server, http.MethodPut, "/api/v1/cluster/host-order", body,
		sessionCookie, csrfCookie, map[string]string{
			"Content-Type": "application/json",
			"Origin":       "http://panel.test",
			"X-CSRF-Token": csrfCookie.Value,
		},
	)
	if updatedResponse.Code != http.StatusOK {
		t.Fatalf("update host order status = %d; body=%s", updatedResponse.Code, updatedResponse.Body.String())
	}
	var updated clusterHostOrderResponse
	if err := json.Unmarshal(updatedResponse.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if !updated.Configured || strings.Join(updated.IDs, ",") != "remote-b,local,remote-a" || updated.ResourceVersion == input.ExpectedResourceVersion {
		t.Fatalf("unexpected updated host order: %#v", updated)
	}

	readResponse := authenticatedRequest(
		server, http.MethodGet, "/api/v1/cluster/host-order", nil,
		sessionCookie, csrfCookie, nil,
	)
	if readResponse.Code != http.StatusOK {
		t.Fatalf("read host order status = %d; body=%s", readResponse.Code, readResponse.Body.String())
	}
	var read clusterHostOrderResponse
	if err := json.Unmarshal(readResponse.Body.Bytes(), &read); err != nil {
		t.Fatal(err)
	}
	if !read.Configured || read.ResourceVersion != updated.ResourceVersion || strings.Join(read.IDs, ",") != "remote-b,local,remote-a" {
		t.Fatalf("unexpected persisted host order: %#v", read)
	}

	inventoryResponse = authenticatedRequest(
		server, http.MethodGet, "/api/v1/cluster/hosts", nil,
		sessionCookie, csrfCookie, nil,
	)
	if err := json.Unmarshal(inventoryResponse.Body.Bytes(), &inventory); err != nil {
		t.Fatal(err)
	}
	if inventory.HostOrder.ResourceVersion != updated.ResourceVersion || strings.Join(inventory.HostOrder.IDs, ",") != "remote-b,local,remote-a" {
		t.Fatalf("inventory omitted current host order: %#v", inventory.HostOrder)
	}

	events, _, _ := server.store.ListAudit(20, "")
	serialized, err := json.Marshal(events)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(serialized), "cluster.host-order.update") || strings.Contains(string(serialized), "remote-b") {
		t.Fatalf("host order audit is missing or exposed host IDs: %s", serialized)
	}
}

func TestClusterHostOrderRejectsStaleAndInvalidUpdates(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	_, _, initialVersion := server.store.ClusterHostOrder()
	headers := map[string]string{
		"Content-Type": "application/json",
		"Origin":       "http://panel.test",
		"X-CSRF-Token": csrfCookie.Value,
	}

	firstBody, _ := json.Marshal(clusterHostOrderInput{
		IDs: []string{"local"}, ExpectedResourceVersion: initialVersion,
	})
	first := authenticatedRequest(
		server, http.MethodPut, "/api/v1/cluster/host-order", firstBody,
		sessionCookie, csrfCookie, headers,
	)
	if first.Code != http.StatusOK {
		t.Fatalf("initial host order update returned %d %s", first.Code, first.Body.String())
	}

	staleBody, _ := json.Marshal(clusterHostOrderInput{
		IDs: []string{"remote"}, ExpectedResourceVersion: initialVersion,
	})
	stale := authenticatedRequest(
		server, http.MethodPut, "/api/v1/cluster/host-order", staleBody,
		sessionCookie, csrfCookie, headers,
	)
	if stale.Code != http.StatusConflict || !strings.Contains(stale.Body.String(), "cluster_host_order_changed") {
		t.Fatalf("stale host order update returned %d %s", stale.Code, stale.Body.String())
	}

	_, _, currentVersion := server.store.ClusterHostOrder()
	invalidBodies := [][]byte{
		[]byte(`{"expectedResourceVersion":"` + currentVersion + `"}`),
		[]byte(`{"ids":["duplicate","duplicate"],"expectedResourceVersion":"` + currentVersion + `"}`),
	}
	for _, invalidBody := range invalidBodies {
		response := authenticatedRequest(
			server, http.MethodPut, "/api/v1/cluster/host-order", invalidBody,
			sessionCookie, csrfCookie, headers,
		)
		if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "cluster_host_order_invalid") {
			t.Fatalf("invalid host order update returned %d %s", response.Code, response.Body.String())
		}
	}
}
