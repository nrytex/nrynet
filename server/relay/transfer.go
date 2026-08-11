package relay

import (
	"context"
	"errors"
	"io"
	"net"
)

type DataStream interface {
	io.Reader
	io.Writer
	io.Closer
}

type copyResult struct {
	n   int64
	err error
}

func (b *Broker) RelayStream(dataConn DataStream, visitor net.Conn, onComplete CompleteFunc) error {
	return b.relay(dataConn, visitor, onComplete)
}

func (b *Broker) relay(dataConn DataStream, visitor net.Conn, onComplete CompleteFunc) error {
	defer dataConn.Close()
	defer visitor.Close()
	uploadCh := make(chan copyResult, 1)
	downloadCh := make(chan copyResult, 1)
	go copyMeasured(dataConn, visitor, uploadCh, b.meter)
	go copyMeasured(visitor, dataConn, downloadCh, b.meter)
	upload, download, err := waitCopies(dataConn, visitor, uploadCh, downloadCh)
	if onComplete != nil {
		onComplete(upload, download)
	}
	return err
}

func waitCopies(
	dataConn DataStream,
	visitor net.Conn,
	uploadCh <-chan copyResult,
	downloadCh <-chan copyResult,
) (int64, int64, error) {
	upload, download, firstErr, uploadDone := firstCopy(uploadCh, downloadCh)
	_ = dataConn.Close()
	_ = visitor.Close()
	if !uploadDone {
		upload = <-uploadCh
	} else {
		download = <-downloadCh
	}
	return upload.n, download.n, firstErr
}

func firstCopy(uploadCh, downloadCh <-chan copyResult) (copyResult, copyResult, error, bool) {
	var upload copyResult
	var download copyResult
	select {
	case upload = <-uploadCh:
		return upload, download, normalizeCopyError(upload.err), true
	case download = <-downloadCh:
		return upload, download, normalizeCopyError(download.err), false
	}
}

func copyMeasured(dst io.Writer, src io.Reader, ch chan<- copyResult, meter *bandwidthMeter) {
	n, err := io.Copy(countingWriter{Writer: dst, meter: meter}, src)
	ch <- copyResult{n: n, err: err}
}

type countingWriter struct {
	io.Writer
	meter *bandwidthMeter
}

func (w countingWriter) Write(data []byte) (int, error) {
	written, err := w.Writer.Write(data)
	w.meter.Add(int64(written))
	return written, err
}

func normalizeCopyError(err error) error {
	if err == nil || err == io.EOF || errors.Is(err, io.ErrClosedPipe) ||
		errors.Is(err, net.ErrClosed) || errors.Is(err, context.Canceled) || isExpectedSocketClose(err) {
		return nil
	}
	return err
}
