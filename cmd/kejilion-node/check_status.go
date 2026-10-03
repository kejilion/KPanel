package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/kejilion/kejilion-panel/internal/contract"
	"io"
	"time"
)

func readNodeCheckStatus() *contract.ServiceCheckSummary {
	content, err := readTrustedRuntimeFile(nodeCheckStatusPath, contract.MaxServiceCheckSummaryBytes)
	if err != nil {
		return nil
	}
	return decodeNodeCheckStatus(content, time.Now())
}

func decodeNodeCheckStatus(content []byte, now time.Time) *contract.ServiceCheckSummary {
	if len(content) > contract.MaxServiceCheckSummaryBytes {
		return nil
	}
	var summary contract.ServiceCheckSummary
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&summary) != nil {
		return nil
	}
	var trailing any
	if !errors.Is(decoder.Decode(&trailing), io.EOF) || !contract.ValidServiceCheckSummary(&summary, now) {
		return nil
	}
	return &summary
}
