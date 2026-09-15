package panel

import (
	"context"
	"errors"
	"net/http"

	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/store"
)

type clusterHostOrderInput struct {
	IDs                     []string `json:"ids"`
	ExpectedResourceVersion string   `json:"expectedResourceVersion"`
}

type clusterHostOrderResponse struct {
	IDs             []string `json:"ids"`
	Configured      bool     `json:"configured"`
	ResourceVersion string   `json:"resourceVersion"`
}

type clusterHostsResponse struct {
	cluster.HostList
	HostOrder clusterHostOrderResponse `json:"hostOrder"`
}

func (s *Server) clusterHostsView(ctx context.Context) clusterHostsResponse {
	hosts := s.cluster.Hosts(ctx)
	value, configured, version := s.store.ClusterHostOrder()
	return clusterHostsResponse{
		HostList:  hosts,
		HostOrder: clusterHostOrderView(value, configured, version),
	}
}

func (s *Server) handleClusterHostOrder(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if _, _, ok := s.requireSession(w, r); !ok {
			return
		}
		value, configured, version := s.store.ClusterHostOrder()
		s.writeJSON(w, http.StatusOK, clusterHostOrderView(value, configured, version))
	case http.MethodPut:
		s.handleClusterHostOrderUpdate(w, r)
	default:
		s.writeProblem(w, r, http.StatusNotFound, "route_not_found", "Route not found", "")
	}
}

func (s *Server) handleClusterHostOrderUpdate(w http.ResponseWriter, r *http.Request) {
	session, ok := s.requireClusterMutation(w, r)
	if !ok {
		return
	}
	var input clusterHostOrderInput
	if err := s.decodeJSON(w, r, &input); err != nil {
		return
	}
	if input.IDs == nil || store.ValidateClusterHostOrder(input.IDs) != nil {
		s.writeProblem(w, r, http.StatusUnprocessableEntity, "cluster_host_order_invalid", "Cluster host order is invalid", "")
		return
	}
	change := map[string]any{"hostCount": len(input.IDs)}
	if err := s.audit(r, session.User.ID, "cluster.host-order.update", "panel-preference", "cluster-host-order", "intent", change); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	value := store.ClusterHostOrder{IDs: append([]string{}, input.IDs...)}
	if err := s.store.ReplaceClusterHostOrder(input.ExpectedResourceVersion, value); err != nil {
		_ = s.audit(r, session.User.ID, "cluster.host-order.update", "panel-preference", "cluster-host-order", "failure", change)
		s.writeClusterHostOrderStoreError(w, r, err)
		return
	}
	_ = s.audit(r, session.User.ID, "cluster.host-order.update", "panel-preference", "cluster-host-order", "success", change)
	current, configured, version := s.store.ClusterHostOrder()
	s.writeJSON(w, http.StatusOK, clusterHostOrderView(current, configured, version))
}

func clusterHostOrderView(value store.ClusterHostOrder, configured bool, version string) clusterHostOrderResponse {
	return clusterHostOrderResponse{
		IDs:             append([]string{}, value.IDs...),
		Configured:      configured,
		ResourceVersion: version,
	}
}

func (s *Server) writeClusterHostOrderStoreError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, store.ErrConflict):
		s.writeProblem(w, r, http.StatusConflict, "cluster_host_order_changed", "Cluster host order changed", "")
	case errors.Is(err, store.ErrInvalidRecord):
		s.writeProblem(w, r, http.StatusUnprocessableEntity, "cluster_host_order_invalid", "Cluster host order is invalid", "")
	default:
		s.writeProblem(w, r, http.StatusServiceUnavailable, "cluster_host_order_storage_unavailable", "Cluster host order storage unavailable", "")
	}
}
