// Package oggreader implements the Ogg media container reader
package oggreader

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

const (
	pageHeaderTypeBeginningOfStream = 0x02
	idPageSignature                 = "OpusHead"
	commentPageSignature            = "OpusTags"
	pageHeaderSignature             = "OggS"

	pageHeaderLen = 28
)

// OggReader is used to read Ogg files and return page payloads
type OggReader struct {
	stream               io.ReadSeeker
	bytesReadSuccesfully int64
}

// OggHeader is the metadata from the first two pages
// in the file (ID and Comment)
//
// https://tools.ietf.org/html/rfc7845.html#section-3
type OggHeader struct {
	ChannelMap uint8
	Channels   uint8
	OutputGain uint16
	PreSkip    uint16
	SampleRate uint32
	Version    uint8
}

// OggPageHeader is the metadata for a Page
// Pages are the fundamental unit of multiplexing in an Ogg stream
//
// https://tools.ietf.org/html/rfc7845.html#section-1
type OggPageHeader struct {
	sig           [4]byte
	version       uint8
	headerType    uint8
	granulePos    uint64
	serial        uint32
	index         uint32
	segmentsCount uint8

	checksum uint32
}

// NewWith returns a new Ogg reader and Ogg header
// with an io.ReadSeeker input
func NewWith(in io.ReadSeeker) (*OggReader, *OggHeader, error) {
	if in == nil {
		return nil, nil, fmt.Errorf("stream is nil")
	}

	reader := &OggReader{
		stream: in,
	}

	header, err := reader.readHeaders()
	if err != nil {
		return nil, nil, err
	}

	return reader, header, nil
}

// read
func (o *OggReader) readHeaders() (*OggHeader, error) {
	payload, pageHeader, err := o.ParseNextPage()
	if err != nil {
		return nil, err
	}

	header := &OggHeader{}
	if string(pageHeader.sig[:]) != pageHeaderSignature {
		return nil, fmt.Errorf("bad header signature")
	}

	if pageHeader.headerType != pageHeaderTypeBeginningOfStream {
		return nil, fmt.Errorf("wrong header, expected beginning of stream")
	}

	// TODO make sure payload is big enough
	if len(payload) == 0 {
		return nil, fmt.Errorf("bad header size")
	}

	sig := payload[:8]
	if s := string(sig); s != idPageSignature {
		return nil, fmt.Errorf("wrong signature: %s", s)
	}

	header.Version = payload[8]
	header.Channels = payload[9]
	header.PreSkip = binary.LittleEndian.Uint16(payload[10:12])
	header.SampleRate = binary.LittleEndian.Uint32(payload[12:16])
	header.OutputGain = binary.LittleEndian.Uint16(payload[16:18])
	header.ChannelMap = payload[18]

	// read and skip comment header pages
	for {
		commentPayload, _, err := o.ParseNextPage()
		if err != nil {
			return nil, err
		}

		// If page was not a header rewind
		if !bytes.Contains(commentPayload, []byte(commentPageSignature)) {
			if _, err = o.stream.Seek(-1*int64(pageHeaderLen+len(commentPayload)), io.SeekCurrent); err != nil {
				return nil, err
			}
			break
		}
	}

	return header, nil
}

// ParseNextPage reads from stream and returns Ogg page payload, header,
// and an error if there is incomplete page data.
func (o *OggReader) ParseNextPage() ([]byte, *OggPageHeader, error) {
	h := make([]byte, pageHeaderLen)

	n, err := o.stream.Read(h)
	if err != nil {
		return nil, nil, err
	} else if n < len(h) {
		return nil, nil, fmt.Errorf("header len mismatch")
	}

	pageHeader := &OggPageHeader{
		sig: [4]byte{h[0], h[1], h[2], h[3]},
	}

	pageHeader.version = h[4]
	pageHeader.headerType = h[5]
	pageHeader.granulePos = binary.LittleEndian.Uint64(h[6 : 6+8])
	pageHeader.serial = binary.LittleEndian.Uint32(h[14 : 14+4])
	pageHeader.index = binary.LittleEndian.Uint32(h[18 : 18+4])
	pageHeader.segmentsCount = h[26]

	payloadSize := h[27]
	payload := []byte{}

	if payloadSize > 0 {
		payload = make([]byte, payloadSize)

		if _, err = o.stream.Read(payload); err != nil {
			return nil, nil, err
		}

		pageHeader.checksum = binary.LittleEndian.Uint32(h[22 : 22+4])
	}

	return payload, pageHeader, nil
}

// ResetReader resets the internal stream of OggReader. This is useful
// for live streams, where the end of the file might be read without the
// data being finished.
func (o *OggReader) ResetReader(reset func(bytesRead int64) io.ReadSeeker) {
	o.stream = reset(o.bytesReadSuccesfully)
}
