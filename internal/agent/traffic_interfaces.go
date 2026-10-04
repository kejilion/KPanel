package agent

import (
	"errors"
	"net/http"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/systeminfo"
)

// maxTrafficInterfacesRequestBytes covers two full name lists and a resource
// version with ample margin.
const maxTrafficInterfacesRequestBytes = 4 << 10

func (s *Server) trafficInterfaces(w http.ResponseWriter, r *http.Request, requestID string) {
	if r.Method != http.MethodGet && r.Method != http.MethodPut {
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodPut)
		writeProblem(w, requestID, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不允许", "")
		return
	}
	if r.URL.RawPath != "" || r.URL.RawQuery != "" || r.ContentLength > maxTrafficInterfacesRequestBytes {
		writeProblem(w, requestID, http.StatusBadRequest, "traffic_interfaces_request_invalid", "流量统计网卡请求无效", "")
		return
	}
	var (
		snapshot contract.TrafficInterfacesSnapshot
		err      error
	)
	if r.Method == http.MethodGet {
		if r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
			writeProblem(w, requestID, http.StatusBadRequest, "traffic_interfaces_request_invalid", "流量统计网卡请求无效", "")
			return
		}
		snapshot, err = s.system.TrafficInterfaces()
	} else {
		r.Body = http.MaxBytesReader(w, r.Body, maxTrafficInterfacesRequestBytes)
		var input contract.UpdateTrafficInterfacesInput
		if decodeJSON(w, r, &input) != nil {
			writeProblem(w, requestID, http.StatusBadRequest, "traffic_interfaces_request_invalid", "流量统计网卡请求无效", "")
			return
		}
		snapshot, err = s.system.ReplaceTrafficInterfaces(input)
	}
	if err != nil {
		var validation *systeminfo.TrafficSelectionValidationError
		switch {
		case errors.As(err, &validation):
			writeProblem(w, requestID, http.StatusUnprocessableEntity, "validation_failed", "Validation failed", validation.Detail)
		case errors.Is(err, systeminfo.ErrTrafficSelectionConflict):
			writeProblem(w, requestID, http.StatusConflict, "traffic_interfaces_changed", "流量统计网卡已被修改", "")
		case errors.Is(err, systeminfo.ErrTrafficSelectionUnavailable):
			writeProblem(w, requestID, http.StatusServiceUnavailable, "traffic_interfaces_unavailable", "流量统计网卡设置不可用", "")
		case r.Method == http.MethodGet:
			writeProblem(w, requestID, http.StatusServiceUnavailable, "traffic_interfaces_unavailable", "无法读取网卡流量计数", "")
		default:
			writeProblem(w, requestID, http.StatusInternalServerError, "traffic_interfaces_update_failed", "流量统计网卡保存失败", "")
		}
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}
