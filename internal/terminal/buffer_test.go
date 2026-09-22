package terminal

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestBufferAppendSkipsOverlapAndMarksGaps(t *testing.T) {
	buffer := NewBuffer(8, 10)
	buffer.Append(10, []byte("abcd"))
	buffer.Append(12, []byte("cdef"))
	output, err := buffer.Output(context.Background(), 10, 0)
	if err != nil || string(output.Data) != "abcdef" || output.NextOffset != 16 || output.Truncated {
		t.Fatalf("overlap output = %+v, %v", output, err)
	}
	buffer.Append(20, []byte("xy"))
	output, err = buffer.Output(context.Background(), 16, 0)
	if err != nil || !output.Truncated || output.Offset != 20 || string(output.Data) != "xy" {
		t.Fatalf("gap output = %+v, %v", output, err)
	}
	if _, err := buffer.Output(context.Background(), 23, 0); !errors.Is(err, ErrOffset) {
		t.Fatalf("future offset error = %v", err)
	}
}

func TestBufferLimitDropsOldestBytes(t *testing.T) {
	buffer := NewBuffer(4, 0)
	buffer.Append(0, []byte("abcdef"))
	output, err := buffer.Output(context.Background(), 0, 0)
	if err != nil || !output.Truncated || output.Offset != 2 || string(output.Data) != "cdef" {
		t.Fatalf("limited output = %+v, %v", output, err)
	}
}

func TestBufferOutputWaitsForAppendAndState(t *testing.T) {
	buffer := NewBuffer(16, 0)
	go func() {
		time.Sleep(20 * time.Millisecond)
		buffer.Append(0, []byte("hi"))
	}()
	output, err := buffer.Output(context.Background(), 0, time.Second)
	if err != nil || string(output.Data) != "hi" {
		t.Fatalf("waited output = %+v, %v", output, err)
	}
	now := time.Now()
	go func() {
		time.Sleep(20 * time.Millisecond)
		buffer.SetState(&now, "exit status 1", false)
	}()
	output, err = buffer.Output(context.Background(), 2, time.Second)
	if err != nil || output.ExitedAt == nil || output.ExitError != "exit status 1" || !buffer.Finished() {
		t.Fatalf("exit output = %+v, %v", output, err)
	}
}

func TestBufferFailureOnlySurfacesWithoutData(t *testing.T) {
	buffer := NewBuffer(16, 0)
	buffer.Append(0, []byte("ok"))
	failure := errors.New("disconnected")
	buffer.Fail(failure)
	output, err := buffer.Output(context.Background(), 0, 0)
	if err != nil || string(output.Data) != "ok" {
		t.Fatalf("buffered output during failure = %+v, %v", output, err)
	}
	if _, err := buffer.Output(context.Background(), 2, time.Second); !errors.Is(err, failure) {
		t.Fatalf("failure error = %v", err)
	}
	buffer.Recover()
	output, err = buffer.Output(context.Background(), 2, 0)
	if err != nil || len(output.Data) != 0 {
		t.Fatalf("recovered output = %+v, %v", output, err)
	}
}
