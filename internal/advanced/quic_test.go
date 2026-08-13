package advanced

import (
	"bytes"
	"context"
	"crypto/x509"
	"net"
	"testing"
	"time"
)

func TestQUICAuthenticatedStreamsCarryFrames(t *testing.T) {
	cert, err := SelfSignedCertificate()
	if err != nil {
		t.Fatal(err)
	}
	server, err := ListenQUIC("127.0.0.1:0", ServerTLSConfig(cert), testAuthenticator)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	serverErr := make(chan error, 1)
	go func() { serverErr <- serveQUICEcho(ctx, server) }()

	certificate, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(certificate)
	_, port, _ := net.SplitHostPort(server.Addr().String())
	clientTLS := ClientTLSConfig("localhost", false)
	clientTLS.RootCAs = roots
	client, err := DialQUIC(ctx, net.JoinHostPort("localhost", port), clientTLS, AuthRequest{
		Token: "secret", DeviceID: "agent-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	stream, err := client.OpenStream(ctx, FrameData)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("hello over quic")
	if err := WriteFrame(stream, Frame{Kind: FrameData, TunnelID: "tun-1", Payload: want}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadFrame(stream)
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != FrameData || !bytes.Equal(got.Payload, want) {
		t.Fatalf("unexpected frame: %#v", got)
	}
	if err := <-serverErr; err != nil {
		t.Fatal(err)
	}
}

func TestQUICConfigAllowsConcurrentDataStreams(t *testing.T) {
	config := quicConfig()
	if config.MaxIncomingStreams < 4096 {
		t.Fatalf("MaxIncomingStreams=%d, want at least 512", config.MaxIncomingStreams)
	}
	if config.MaxIncomingUniStreams < 256 {
		t.Fatalf("MaxIncomingUniStreams=%d, want at least 64", config.MaxIncomingUniStreams)
	}
}

func testAuthenticator(_ context.Context, request AuthRequest, _ net.Addr) error {
	if request.Token != "secret" || request.DeviceID != "agent-1" {
		return errUnauthorizedForTest{}
	}
	return nil
}

func serveQUICEcho(ctx context.Context, server *QUICServer) error {
	session, err := server.Accept(ctx)
	if err != nil {
		return err
	}
	defer session.Close()
	stream, err := session.AcceptStream(ctx)
	if err != nil {
		return err
	}
	defer stream.Close()
	frame, err := ReadFrame(stream)
	if err != nil {
		return err
	}
	if err := WriteFrame(stream, frame); err != nil {
		return err
	}
	time.Sleep(50 * time.Millisecond)
	return nil
}

type errUnauthorizedForTest struct{}

func (errUnauthorizedForTest) Error() string { return "unauthorized" }
