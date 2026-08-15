package receive

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/canta-9142/qshare/internal/share"
)

const TextQueueCapacity = 16

var ErrTextProcessorClosed = errors.New("text processor is closed")

type TextSink interface {
	WriteText(context.Context, share.Text) error
}

type textSubmission struct {
	text   share.Text
	result chan error
}

// TextProcessor processes validated text submissions serially through a
// bounded FIFO queue.
type TextProcessor struct {
	ctx         context.Context
	cancel      context.CancelFunc
	submissions chan textSubmission
	done        chan struct{}
	closeOnce   sync.Once
}

func NewTextProcessor(sink TextSink, capacity int) *TextProcessor {
	if capacity <= 0 {
		panic("text queue capacity must be positive")
	}

	ctx, cancel := context.WithCancel(context.Background())
	processor := &TextProcessor{
		ctx:         ctx,
		cancel:      cancel,
		submissions: make(chan textSubmission, capacity),
		done:        make(chan struct{}),
	}
	go processor.run(sink)
	return processor
}

func (p *TextProcessor) Submit(ctx context.Context, text share.Text) error {
	result := make(chan error, 1)
	submission := textSubmission{text: text, result: result}

	select {
	case p.submissions <- submission:
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-p.ctx.Done():
		return ErrTextProcessorClosed
	}

	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-p.ctx.Done():
		return ErrTextProcessorClosed
	}
}

func (p *TextProcessor) Close() {
	p.closeOnce.Do(p.cancel)
	<-p.done
}

func (p *TextProcessor) run(sink TextSink) {
	defer close(p.done)

	for {
		select {
		case submission := <-p.submissions:
			err := sink.WriteText(p.ctx, submission.text)
			submission.result <- err
		case <-p.ctx.Done():
			return
		}
	}
}

type WriterTextSink struct {
	destination io.Writer
}

func NewWriterTextSink(destination io.Writer) *WriterTextSink {
	return &WriterTextSink{destination: destination}
}

func (s *WriterTextSink) WriteText(ctx context.Context, text share.Text) error {
	if err := context.Cause(ctx); err != nil {
		return err
	}

	value := text.Bytes()
	for len(value) > 0 {
		written, err := s.destination.Write(value)
		if err != nil {
			return fmt.Errorf("write received text: %w", err)
		}
		if written == 0 {
			return fmt.Errorf("write received text: %w", io.ErrShortWrite)
		}
		value = value[written:]
	}
	return nil
}
