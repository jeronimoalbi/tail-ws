package watch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
	"golang.org/x/sync/errgroup"
)

var (
	ErrFileDeleted     = errors.New("watched file has been deleted")
	ErrFileOverwritten = errors.New("watched file has been overwritten")
	ErrFileRenamed     = errors.New("watched file has been renamed")
)

// Watcher defines the interface for trasaction head file watchers.
type Watcher interface {
	// Watch starts watching a transaction head file for changer.
	Watch(ctx context.Context, name string) error
}

// OffsetGetter defines the interface for transaction head file offset getters.
type OffsetGetter interface {
	// GetOffset returns the current transaction head file offset.
	GetOffset() int64
}

// Reader defines the interface for transaction head file readers.
type Reader interface {
	io.ReadCloser
	Watcher
	OffsetGetter
}

// Option configures watch readers.
type Option func(*reader)

// StartOffset sets the initial offset for the reader.
func StartOffset(offset int64) Option {
	return func(r *reader) {
		r.offset = offset
	}
}

// SeekEnd points the reader to the end of the contents.
func SeekEnd() Option {
	return func(r *reader) {
		r.seekEnd = true
	}
}

// NewReader creates a new transaction head file reader.
func NewReader(options ...Option) Reader {
	r := &reader{
		read: make(chan struct{}),
	}

	for _, apply := range options {
		apply(r)
	}

	return r
}

type reader struct {
	offset  int64
	seekEnd bool
	file    io.ReadSeekCloser
	read    chan struct{}
}

func (r *reader) Read(b []byte) (n int, err error) {
	// Block until watch is called
	if _, ok := <-r.read; !ok {
		// Channel closed before the first read
		return 0, io.EOF
	}

	for {
		n, err := r.file.Read(b)
		if err != nil && err != io.EOF {
			return n, err
		}

		// On successful read increment offset
		if n > 0 {
			r.offset += int64(n)
			return n, nil
		}

		// When read fails with EOF wait until more content is available
		if _, ok := <-r.read; !ok {
			// Finish reading when read channel is closed
			return 0, io.EOF
		}
	}
}

func (r *reader) Close() error {
	close(r.read)
	return nil
}

func (r *reader) GetOffset() int64 {
	return r.offset
}

func (r *reader) Watch(ctx context.Context, name string) error {
	name, err := filepath.Abs(name)
	if err != nil {
		return err
	}

	r.file, err = r.openFile(name)
	if err != nil {
		return err
	}

	defer func() {
		r.file.Close()
	}()

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	defer watcher.Close()

	dir := filepath.Dir(name)
	if err := watcher.Add(dir); err != nil {
		return err
	}

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		for {
			select {
			case e := <-watcher.Events:
				if e.Name != name {
					continue
				}

				if err := r.handleEvent(e); err != nil {
					return err
				}
			case err := <-watcher.Errors:
				if err != nil {
					return err
				}
			case <-ctx.Done():
				return nil
			}
		}
	})

	return g.Wait()
}

func (r *reader) openFile(name string) (io.ReadSeekCloser, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}

	if r.offset > 0 {
		if _, err := f.Seek(r.offset, io.SeekStart); err != nil {
			return nil, fmt.Errorf("error seeking file offset %d: %w", r.offset, err)
		}
	} else if r.seekEnd {
		if _, err := f.Seek(0, io.SeekEnd); err != nil {
			return nil, fmt.Errorf("error seeking file end: %w", err)
		}
	}

	return f, nil
}

func (r *reader) handleEvent(e fsnotify.Event) error {
	if e.Has(fsnotify.Remove) {
		if _, err := os.Stat(e.Name); os.IsNotExist(err) {
			return ErrFileDeleted
		}
		return ErrFileOverwritten
	}

	if e.Has(fsnotify.Rename) {
		return ErrFileRenamed
	}

	// On write keep reading the file
	if e.Has(fsnotify.Write) {
		info, err := os.Stat(e.Name)
		if err != nil {
			return err
		}

		// Reset offset when file is truncated
		if size := info.Size(); size <= r.offset {
			if _, err := r.file.Seek(0, io.SeekStart); err != nil {
				return err
			}

			r.offset = 0

			// Ignore empty truncated file events
			if size == 0 {
				return nil
			}
		}

		r.read <- struct{}{}
	}

	return nil
}
