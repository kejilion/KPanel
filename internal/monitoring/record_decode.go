package monitoring

import "encoding/json"

// diskRecordDecoder reuses bounded scratch space within one scan. Callers must
// consume returned slices before the next decode; retained points/metadata must
// be copied by value. Strings are still owned by encoding/json, never Scanner.
type diskRecordDecoder struct {
	record     diskRecord
	containers []diskContainerPoint
	latency    []diskOperatorLatencyPoint
}

func (d *diskRecordDecoder) decode(content []byte) (diskRecord, error) {
	if d.containers == nil {
		d.containers = make([]diskContainerPoint, maxHistorySeries)
		d.latency = make([]diskOperatorLatencyPoint, MaxChecks)
	}
	// Clear the entire backing arrays, including elements left by a shorter or
	// invalid previous row. Otherwise omitted fields and null array elements
	// could inherit another sample's values through encoding/json's slice reuse.
	clear(d.containers)
	clear(d.latency)
	d.record = diskRecord{Containers: d.containers[:0], OperatorLatency: d.latency[:0]}
	err := json.Unmarshal(content, &d.record)
	// Unusual legacy rows may exceed normal series limits. Decode them exactly
	// as before, but do not retain their potentially large arrays for the scan.
	if capacity := cap(d.record.Containers); capacity > 0 && capacity <= maxScannedSeries {
		d.containers = d.record.Containers[:capacity]
	}
	if capacity := cap(d.record.OperatorLatency); capacity > 0 && capacity <= MaxChecks {
		d.latency = d.record.OperatorLatency[:capacity]
	}
	return d.record, err
}
