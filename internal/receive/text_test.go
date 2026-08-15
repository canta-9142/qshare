package receive

import (
	"bytes"
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/canta-9142/qshare/internal/share"
)

func TestTextProcessorSerializesSubmissionsInQueueOrder(t *testing.T) {
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	var mu sync.Mutex
	var values []string
	sink := textSinkFunc(func(_ context.Context, text share.Text) error {
		if text.String() == "first" {
			close(firstStarted)
			<-releaseFirst
		}
		mu.Lock()
		values = append(values, text.String())
		mu.Unlock()
		return nil
	})
	processor := NewTextProcessor(sink, 2)
	t.Cleanup(processor.Close)

	firstDone := submitAsync(processor, textForTest(t, "first"))
	<-firstStarted
	secondDone := submitAsync(processor, textForTest(t, "second"))
	waitForQueueLength(t, processor, 1)
	thirdDone := submitAsync(processor, textForTest(t, "third"))
	waitForQueueLength(t, processor, 2)
	close(releaseFirst)

	for _, done := range []<-chan error{firstDone, secondDone, thirdDone} {
		if err := <-done; err != nil {
			t.Fatalf("Submit() error = %v", err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if got := values; len(got) != 3 || got[0] != "first" || got[1] != "second" || got[2] != "third" {
		t.Fatalf("processed values = %q, want [first second third]", got)
	}
}

func TestTextProcessorWaitsForSinkResultAndContinuesAfterFailure(t *testing.T) {
	want := errors.New("sink failed")
	calls := 0
	processor := NewTextProcessor(textSinkFunc(func(context.Context, share.Text) error {
		calls++
		if calls == 1 {
			return want
		}
		return nil
	}), 1)
	t.Cleanup(processor.Close)

	if err := processor.Submit(context.Background(), textForTest(t, "first")); !errors.Is(err, want) {
		t.Fatalf("first Submit() error = %v, want sink error", err)
	}
	if err := processor.Submit(context.Background(), textForTest(t, "second")); err != nil {
		t.Fatalf("second Submit() error = %v", err)
	}
	if calls != 2 {
		t.Fatalf("sink calls = %d, want 2", calls)
	}
}

func TestTextProcessorAppliesBoundedBackpressure(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	processor := NewTextProcessor(textSinkFunc(func(context.Context, share.Text) error {
		select {
		case <-started:
		default:
			close(started)
		}
		<-release
		return nil
	}), 1)
	t.Cleanup(processor.Close)

	first := submitAsync(processor, textForTest(t, "first"))
	<-started
	second := submitAsync(processor, textForTest(t, "second"))
	waitForQueueLength(t, processor, 1)
	thirdCtx, cancelThird := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancelThird()
	if err := processor.Submit(thirdCtx, textForTest(t, "third")); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("third Submit() error = %v, want deadline exceeded", err)
	}

	close(release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if err := <-second; err != nil {
		t.Fatal(err)
	}
}

func TestWriterTextSinkPreservesBytesWithoutSeparators(t *testing.T) {
	var destination bytes.Buffer
	sink := NewWriterTextSink(&destination)
	for _, value := range []string{"first\x00\r\n", "第二"} {
		if err := sink.WriteText(context.Background(), textForTest(t, value)); err != nil {
			t.Fatalf("WriteText() error = %v", err)
		}
	}
	if got, want := destination.String(), "first\x00\r\n第二"; got != want {
		t.Fatalf("destination = %q, want %q", got, want)
	}
}

type textSinkFunc func(context.Context, share.Text) error

func (function textSinkFunc) WriteText(ctx context.Context, text share.Text) error {
	return function(ctx, text)
}

func submitAsync(processor *TextProcessor, text share.Text) <-chan error {
	done := make(chan error, 1)
	go func() {
		done <- processor.Submit(context.Background(), text)
	}()
	return done
}

func textForTest(t *testing.T, value string) share.Text {
	t.Helper()
	text, err := share.NewText([]byte(value))
	if err != nil {
		t.Fatal(err)
	}
	return text
}

func waitForQueueLength(t *testing.T, processor *TextProcessor, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for len(processor.submissions) != want {
		if time.Now().After(deadline) {
			t.Fatalf("queue length = %d, want %d", len(processor.submissions), want)
		}
		runtime.Gosched()
	}
}
