package gateway

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
)

const maxHTTPHeader = 64 * 1024

func sniffConnection(conn net.Conn) (net.Conn, string, string, error) {
	reader := bufio.NewReaderSize(conn, maxHTTPHeader)
	first, err := reader.Peek(1)
	if err != nil {
		return nil, "", "", err
	}
	if first[0] == 0x16 {
		prefix, domain, err := sniffTLS(reader)
		return newPrefixedConn(conn, prefix, reader), "https", domain, err
	}
	prefix, domain, err := sniffHTTP(reader)
	return newPrefixedConn(conn, prefix, reader), "http", domain, err
}

func sniffHTTP(reader *bufio.Reader) ([]byte, string, error) {
	var captured bytes.Buffer
	tee := bufio.NewReader(io.TeeReader(reader, &captured))
	request, err := http.ReadRequest(tee)
	if err != nil {
		return nil, "", fmt.Errorf("read HTTP request: %w", err)
	}
	_ = request.Body.Close()
	host := request.Host
	if parsed, _, err := net.SplitHostPort(host); err == nil {
		host = parsed
	}
	if host == "" {
		return nil, "", errors.New("HTTP Host header is required")
	}
	return captured.Bytes(), normalizeDomain(host), nil
}

func sniffTLS(reader *bufio.Reader) ([]byte, string, error) {
	header, err := reader.Peek(5)
	if err != nil {
		return nil, "", err
	}
	recordLength := int(binary.BigEndian.Uint16(header[3:5]))
	if recordLength < 5 || recordLength > maxHTTPHeader {
		return nil, "", errors.New("invalid TLS ClientHello length")
	}
	record, err := reader.Peek(5 + recordLength)
	if err != nil {
		return nil, "", err
	}
	prefix := append([]byte(nil), record...)
	_, _ = reader.Discard(len(record))
	domain, err := parseServerName(record[5:])
	return prefix, normalizeDomain(domain), err
}

func parseServerName(handshake []byte) (string, error) {
	if len(handshake) < 4 || handshake[0] != 0x01 {
		return "", errors.New("TLS record is not a ClientHello")
	}
	body := handshake[4:]
	if len(body) < 34 {
		return "", errors.New("truncated TLS ClientHello")
	}
	position := 34
	position, err := skipVector(body, position, 1)
	if err != nil {
		return "", err
	}
	position, err = skipVector(body, position, 2)
	if err != nil {
		return "", err
	}
	position, err = skipVector(body, position, 1)
	if err != nil || position+2 > len(body) {
		return "", errors.New("truncated TLS extensions")
	}
	extensionsLength := int(binary.BigEndian.Uint16(body[position : position+2]))
	position += 2
	end := position + extensionsLength
	if end > len(body) {
		return "", errors.New("truncated TLS extensions")
	}
	return findServerName(body[position:end])
}

func skipVector(data []byte, position, lengthBytes int) (int, error) {
	if position+lengthBytes > len(data) {
		return 0, errors.New("truncated TLS vector")
	}
	length := int(data[position])
	if lengthBytes == 2 {
		length = int(binary.BigEndian.Uint16(data[position : position+2]))
	}
	position += lengthBytes + length
	if position > len(data) {
		return 0, errors.New("truncated TLS vector")
	}
	return position, nil
}

func findServerName(extensions []byte) (string, error) {
	for len(extensions) >= 4 {
		kind := binary.BigEndian.Uint16(extensions[:2])
		length := int(binary.BigEndian.Uint16(extensions[2:4]))
		if 4+length > len(extensions) {
			break
		}
		if kind == 0 {
			return decodeServerName(extensions[4 : 4+length])
		}
		extensions = extensions[4+length:]
	}
	return "", errors.New("TLS ClientHello has no SNI")
}

func decodeServerName(data []byte) (string, error) {
	if len(data) < 5 {
		return "", errors.New("invalid SNI extension")
	}
	nameLength := int(binary.BigEndian.Uint16(data[3:5]))
	if data[2] != 0 || 5+nameLength > len(data) {
		return "", errors.New("invalid SNI host name")
	}
	return string(data[5 : 5+nameLength]), nil
}

func normalizeDomain(value string) string {
	return strings.ToLower(strings.TrimSuffix(value, "."))
}

type prefixedConn struct {
	net.Conn
	reader io.Reader
}

func newPrefixedConn(conn net.Conn, prefix []byte, tail io.Reader) net.Conn {
	return &prefixedConn{Conn: conn, reader: io.MultiReader(bytes.NewReader(prefix), tail)}
}

func (c *prefixedConn) Read(buffer []byte) (int, error) {
	return c.reader.Read(buffer)
}
