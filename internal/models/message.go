package models

import (
	"encoding/base64"
	"encoding/json"
	"errors"

	"github.com/eclipse-iofog/agent/internal/utils/bytesutil"
)

const (
	// MessageVersion is the current version of the message format
	MessageVersion int16 = 4
)

// Message represents an ioMessage
type Message struct {
	ID               *string `json:"id,omitempty" yaml:"id,omitempty"`
	Tag              *string `json:"tag,omitempty" yaml:"tag,omitempty"`
	MessageGroupID   *string `json:"groupid,omitempty" yaml:"groupid,omitempty"`
	SequenceNumber   int     `json:"sequencenumber" yaml:"sequencenumber"`
	SequenceTotal    int     `json:"sequencetotal" yaml:"sequencetotal"`
	Priority         byte    `json:"priority" yaml:"priority"`
	Timestamp        int64   `json:"timestamp" yaml:"timestamp"`
	Publisher        *string `json:"publisher,omitempty" yaml:"publisher,omitempty"`
	AuthIdentifier   *string `json:"authid,omitempty" yaml:"authid,omitempty"`
	AuthGroup        *string `json:"authgroup,omitempty" yaml:"authgroup,omitempty"`
	Version          int16   `json:"version" yaml:"version"`
	ChainPosition    int64   `json:"chainposition" yaml:"chainposition"`
	Hash             *string `json:"hash,omitempty" yaml:"hash,omitempty"`
	PreviousHash     *string `json:"previoushash,omitempty" yaml:"previoushash,omitempty"`
	Nonce            *string `json:"nonce,omitempty" yaml:"nonce,omitempty"`
	DifficultyTarget int     `json:"difficultytarget" yaml:"difficultytarget"`
	InfoType         *string `json:"infotype,omitempty" yaml:"infotype,omitempty"`
	InfoFormat       *string `json:"infoformat,omitempty" yaml:"infoformat,omitempty"`
	ContextData      []byte  `json:"contextdata,omitempty" yaml:"contextdata,omitempty"`
	ContentData      []byte  `json:"contentdata,omitempty" yaml:"contentdata,omitempty"`
}

// NewMessage creates a new Message with default values
func NewMessage() *Message {
	return &Message{
		Version: MessageVersion,
	}
}

// NewMessageWithPublisher creates a new Message with a publisher
func NewMessageWithPublisher(publisher string) *Message {
	msg := NewMessage()
	msg.Publisher = &publisher
	return msg
}

// UnmarshalJSON custom unmarshaling to handle base64 encoded data and null strings
func (m *Message) UnmarshalJSON(data []byte) error {
	type Alias Message
	aux := &struct {
		ContextData string `json:"contextdata,omitempty"`
		ContentData string `json:"contentdata,omitempty"`
		*Alias
	}{
		Alias: (*Alias)(m),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	// Decode base64 context data
	if aux.ContextData != "" {
		decoded, err := base64.StdEncoding.DecodeString(aux.ContextData)
		if err != nil {
			return err
		}
		m.ContextData = decoded
	}

	// Decode base64 content data
	if aux.ContentData != "" {
		decoded, err := base64.StdEncoding.DecodeString(aux.ContentData)
		if err != nil {
			return err
		}
		m.ContentData = decoded
	}

	return nil
}

// MarshalJSON custom marshaling to handle base64 encoding and null strings
func (m *Message) MarshalJSON() ([]byte, error) {
	type Alias Message
	aux := &struct {
		ID             string `json:"id,omitempty"`
		Tag            string `json:"tag,omitempty"`
		MessageGroupID string `json:"groupid,omitempty"`
		Publisher      string `json:"publisher,omitempty"`
		AuthIdentifier string `json:"authid,omitempty"`
		AuthGroup      string `json:"authgroup,omitempty"`
		Hash           string `json:"hash,omitempty"`
		PreviousHash   string `json:"previoushash,omitempty"`
		Nonce          string `json:"nonce,omitempty"`
		InfoType       string `json:"infotype,omitempty"`
		InfoFormat     string `json:"infoformat,omitempty"`
		ContextData    string `json:"contextdata,omitempty"`
		ContentData    string `json:"contentdata,omitempty"`
		*Alias
	}{
		Alias: (*Alias)(m),
	}

	// Convert pointers to strings (empty string if nil)
	if m.ID != nil {
		aux.ID = *m.ID
	}
	if m.Tag != nil {
		aux.Tag = *m.Tag
	}
	if m.MessageGroupID != nil {
		aux.MessageGroupID = *m.MessageGroupID
	}
	if m.Publisher != nil {
		aux.Publisher = *m.Publisher
	}
	if m.AuthIdentifier != nil {
		aux.AuthIdentifier = *m.AuthIdentifier
	}
	if m.AuthGroup != nil {
		aux.AuthGroup = *m.AuthGroup
	}
	if m.Hash != nil {
		aux.Hash = *m.Hash
	}
	if m.PreviousHash != nil {
		aux.PreviousHash = *m.PreviousHash
	}
	if m.Nonce != nil {
		aux.Nonce = *m.Nonce
	}
	if m.InfoType != nil {
		aux.InfoType = *m.InfoType
	}
	if m.InfoFormat != nil {
		aux.InfoFormat = *m.InfoFormat
	}

	// Encode binary data to base64
	if len(m.ContextData) > 0 {
		aux.ContextData = base64.StdEncoding.EncodeToString(m.ContextData)
	}
	if len(m.ContentData) > 0 {
		aux.ContentData = base64.StdEncoding.EncodeToString(m.ContentData)
	}

	return json.Marshal(aux)
}

// ToJSON converts the message to a JSON object (for compatibility with Java)
func (m *Message) ToJSON() (map[string]interface{}, error) {
	data, err := m.MarshalJSON()
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// GetBytes serializes the message to binary format
// Format: 33-byte header + variable-length data
func (m *Message) GetBytes() ([]byte, error) {
	header := make([]byte, 33)
	data := make([]byte, 0)

	// Version (bytes 0-1)
	versionBytes := bytesutil.ShortToBytes(MessageVersion)
	copy(header[0:], versionBytes)

	// ID (byte 2: length, then data)
	idLen := 0
	if m.ID != nil {
		idLen = len(*m.ID)
	}
	header[2] = byte(idLen)
	if idLen > 0 {
		data = append(data, bytesutil.StringToBytes(*m.ID)...)
	}

	// Tag (bytes 3-4: length as short, then data)
	tagLen := 0
	if m.Tag != nil {
		tagLen = len(*m.Tag)
	}
	tagLenBytes := bytesutil.ShortToBytes(int16(tagLen)) // #nosec G115 -- wire-format: field length fits in int16 by protocol spec
	copy(header[3:], tagLenBytes)
	if tagLen > 0 {
		data = append(data, bytesutil.StringToBytes(*m.Tag)...)
	}

	// MessageGroupID (byte 5: length, then data)
	groupIDLen := 0
	if m.MessageGroupID != nil {
		groupIDLen = len(*m.MessageGroupID)
	}
	header[5] = byte(groupIDLen)
	if groupIDLen > 0 {
		data = append(data, bytesutil.StringToBytes(*m.MessageGroupID)...)
	}

	// SequenceNumber (byte 6: length 0 or 4, then data)
	if m.SequenceNumber == 0 {
		header[6] = 0
	} else {
		header[6] = 4
		data = append(data, bytesutil.IntegerToBytes(m.SequenceNumber)...)
	}

	// SequenceTotal (byte 7: length 0 or 4, then data)
	if m.SequenceTotal == 0 {
		header[7] = 0
	} else {
		header[7] = 4
		data = append(data, bytesutil.IntegerToBytes(m.SequenceTotal)...)
	}

	// Priority (byte 8: length 0 or 1, then data)
	if m.Priority == 0 {
		header[8] = 0
	} else {
		header[8] = 1
		data = append(data, m.Priority)
	}

	// Timestamp (byte 9: length 0 or 8, then data)
	if m.Timestamp == 0 {
		header[9] = 0
	} else {
		header[9] = 8
		data = append(data, bytesutil.LongToBytes(m.Timestamp)...)
	}

	// Publisher (byte 10: length, then data)
	publisherLen := 0
	if m.Publisher != nil {
		publisherLen = len(*m.Publisher)
	}
	header[10] = byte(publisherLen)
	if publisherLen > 0 {
		data = append(data, bytesutil.StringToBytes(*m.Publisher)...)
	}

	// AuthIdentifier (bytes 11-12: length as short, then data)
	authIDLen := 0
	if m.AuthIdentifier != nil {
		authIDLen = len(*m.AuthIdentifier)
	}
	authIDLenBytes := bytesutil.ShortToBytes(int16(authIDLen)) // #nosec G115 -- wire-format: field length fits in int16 by protocol spec
	copy(header[11:], authIDLenBytes)
	if authIDLen > 0 {
		data = append(data, bytesutil.StringToBytes(*m.AuthIdentifier)...)
	}

	// AuthGroup (bytes 13-14: length as short, then data)
	authGroupLen := 0
	if m.AuthGroup != nil {
		authGroupLen = len(*m.AuthGroup)
	}
	authGroupLenBytes := bytesutil.ShortToBytes(int16(authGroupLen)) // #nosec G115 -- wire-format: field length fits in int16 by protocol spec
	copy(header[13:], authGroupLenBytes)
	if authGroupLen > 0 {
		data = append(data, bytesutil.StringToBytes(*m.AuthGroup)...)
	}

	// ChainPosition (byte 15: length 0 or 8, then data)
	if m.ChainPosition == 0 {
		header[15] = 0
	} else {
		header[15] = 8
		data = append(data, bytesutil.LongToBytes(m.ChainPosition)...)
	}

	// Hash (bytes 16-17: length as short, then data)
	hashLen := 0
	if m.Hash != nil {
		hashLen = len(*m.Hash)
	}
	hashLenBytes := bytesutil.ShortToBytes(int16(hashLen)) // #nosec G115 -- wire-format: field length fits in int16 by protocol spec
	copy(header[16:], hashLenBytes)
	if hashLen > 0 {
		data = append(data, bytesutil.StringToBytes(*m.Hash)...)
	}

	// PreviousHash (bytes 18-19: length as short, then data)
	prevHashLen := 0
	if m.PreviousHash != nil {
		prevHashLen = len(*m.PreviousHash)
	}
	prevHashLenBytes := bytesutil.ShortToBytes(int16(prevHashLen)) // #nosec G115 -- wire-format: field length fits in int16 by protocol spec
	copy(header[18:], prevHashLenBytes)
	if prevHashLen > 0 {
		data = append(data, bytesutil.StringToBytes(*m.PreviousHash)...)
	}

	// Nonce (bytes 20-21: length as short, then data)
	nonceLen := 0
	if m.Nonce != nil {
		nonceLen = len(*m.Nonce)
	}
	nonceLenBytes := bytesutil.ShortToBytes(int16(nonceLen)) // #nosec G115 -- wire-format: field length fits in int16 by protocol spec
	copy(header[20:], nonceLenBytes)
	if nonceLen > 0 {
		data = append(data, bytesutil.StringToBytes(*m.Nonce)...)
	}

	// DifficultyTarget (byte 22: length 0 or 4, then data)
	if m.DifficultyTarget == 0 {
		header[22] = 0
	} else {
		header[22] = 4
		data = append(data, bytesutil.IntegerToBytes(m.DifficultyTarget)...)
	}

	// InfoType (byte 23: length, then data)
	infoTypeLen := 0
	if m.InfoType != nil {
		infoTypeLen = len(*m.InfoType)
	}
	header[23] = byte(infoTypeLen)
	if infoTypeLen > 0 {
		data = append(data, bytesutil.StringToBytes(*m.InfoType)...)
	}

	// InfoFormat (byte 24: length, then data)
	infoFormatLen := 0
	if m.InfoFormat != nil {
		infoFormatLen = len(*m.InfoFormat)
	}
	header[24] = byte(infoFormatLen)
	if infoFormatLen > 0 {
		data = append(data, bytesutil.StringToBytes(*m.InfoFormat)...)
	}

	// ContextData (bytes 25-28: length as integer, then data)
	contextDataLen := len(m.ContextData)
	contextDataLenBytes := bytesutil.IntegerToBytes(contextDataLen)
	copy(header[25:], contextDataLenBytes)
	if contextDataLen > 0 {
		data = append(data, m.ContextData...)
	}

	// ContentData (bytes 29-32: length as integer, then data)
	contentDataLen := len(m.ContentData)
	contentDataLenBytes := bytesutil.IntegerToBytes(contentDataLen)
	copy(header[29:], contentDataLenBytes)
	if contentDataLen > 0 {
		data = append(data, m.ContentData...)
	}

	// Combine header and data
	result := make([]byte, 0, 33+len(data))
	result = append(result, header...)
	result = append(result, data...)
	return result, nil
}

// NewMessageFromBytes creates a Message from binary format
func NewMessageFromBytes(rawBytes []byte) (*Message, error) {
	if len(rawBytes) < 33 {
		return nil, errors.New("message too short: need at least 33 bytes for header")
	}

	msg := &Message{}

	// Version (bytes 0-1)
	version := bytesutil.BytesToShort(bytesutil.CopyOfRange(rawBytes, 0, 2))
	if version != MessageVersion {
		return nil, errors.New("incompatible message version")
	}
	msg.Version = version

	pos := 33

	// ID (byte 2: length)
	size := int(rawBytes[2])
	if size > 0 {
		if pos+size > len(rawBytes) {
			return nil, errors.New("message too short for ID field")
		}
		idStr := bytesutil.BytesToString(bytesutil.CopyOfRange(rawBytes, pos, pos+size))
		msg.ID = &idStr
		pos += size
	}

	// Tag (bytes 3-4: length as short)
	size = int(bytesutil.BytesToShort(bytesutil.CopyOfRange(rawBytes, 3, 5)))
	if size > 0 {
		if pos+size > len(rawBytes) {
			return nil, errors.New("message too short for Tag field")
		}
		tagStr := bytesutil.BytesToString(bytesutil.CopyOfRange(rawBytes, pos, pos+size))
		msg.Tag = &tagStr
		pos += size
	}

	// MessageGroupID (byte 5: length)
	size = int(rawBytes[5])
	if size > 0 {
		if pos+size > len(rawBytes) {
			return nil, errors.New("message too short for MessageGroupID field")
		}
		groupIDStr := bytesutil.BytesToString(bytesutil.CopyOfRange(rawBytes, pos, pos+size))
		msg.MessageGroupID = &groupIDStr
		pos += size
	}

	// SequenceNumber (byte 6: length 0 or 4)
	size = int(rawBytes[6])
	if size > 0 {
		if pos+size > len(rawBytes) {
			return nil, errors.New("message too short for SequenceNumber field")
		}
		msg.SequenceNumber = bytesutil.BytesToInteger(bytesutil.CopyOfRange(rawBytes, pos, pos+size))
		pos += size
	}

	// SequenceTotal (byte 7: length 0 or 4)
	size = int(rawBytes[7])
	if size > 0 {
		if pos+size > len(rawBytes) {
			return nil, errors.New("message too short for SequenceTotal field")
		}
		msg.SequenceTotal = bytesutil.BytesToInteger(bytesutil.CopyOfRange(rawBytes, pos, pos+size))
		pos += size
	}

	// Priority (byte 8: length 0 or 1)
	size = int(rawBytes[8])
	if size > 0 {
		if pos+size > len(rawBytes) {
			return nil, errors.New("message too short for Priority field")
		}
		msg.Priority = rawBytes[pos]
		pos += size
	}

	// Timestamp (byte 9: length 0 or 8)
	size = int(rawBytes[9])
	if size > 0 {
		if pos+size > len(rawBytes) {
			return nil, errors.New("message too short for Timestamp field")
		}
		msg.Timestamp = bytesutil.BytesToLong(bytesutil.CopyOfRange(rawBytes, pos, pos+size))
		pos += size
	}

	// Publisher (byte 10: length)
	size = int(rawBytes[10])
	if size > 0 {
		if pos+size > len(rawBytes) {
			return nil, errors.New("message too short for Publisher field")
		}
		publisherStr := bytesutil.BytesToString(bytesutil.CopyOfRange(rawBytes, pos, pos+size))
		msg.Publisher = &publisherStr
		pos += size
	}

	// AuthIdentifier (bytes 11-12: length as short)
	size = int(bytesutil.BytesToShort(bytesutil.CopyOfRange(rawBytes, 11, 13)))
	if size > 0 {
		if pos+size > len(rawBytes) {
			return nil, errors.New("message too short for AuthIdentifier field")
		}
		authIDStr := bytesutil.BytesToString(bytesutil.CopyOfRange(rawBytes, pos, pos+size))
		msg.AuthIdentifier = &authIDStr
		pos += size
	}

	// AuthGroup (bytes 13-14: length as short)
	size = int(bytesutil.BytesToShort(bytesutil.CopyOfRange(rawBytes, 13, 15)))
	if size > 0 {
		if pos+size > len(rawBytes) {
			return nil, errors.New("message too short for AuthGroup field")
		}
		authGroupStr := bytesutil.BytesToString(bytesutil.CopyOfRange(rawBytes, pos, pos+size))
		msg.AuthGroup = &authGroupStr
		pos += size
	}

	// ChainPosition (byte 15: length 0 or 8)
	size = int(rawBytes[15])
	if size > 0 {
		if pos+size > len(rawBytes) {
			return nil, errors.New("message too short for ChainPosition field")
		}
		msg.ChainPosition = bytesutil.BytesToLong(bytesutil.CopyOfRange(rawBytes, pos, pos+size))
		pos += size
	}

	// Hash (bytes 16-17: length as short)
	size = int(bytesutil.BytesToShort(bytesutil.CopyOfRange(rawBytes, 16, 18)))
	if size > 0 {
		if pos+size > len(rawBytes) {
			return nil, errors.New("message too short for Hash field")
		}
		hashStr := bytesutil.BytesToString(bytesutil.CopyOfRange(rawBytes, pos, pos+size))
		msg.Hash = &hashStr
		pos += size
	}

	// PreviousHash (bytes 18-19: length as short)
	size = int(bytesutil.BytesToShort(bytesutil.CopyOfRange(rawBytes, 18, 20)))
	if size > 0 {
		if pos+size > len(rawBytes) {
			return nil, errors.New("message too short for PreviousHash field")
		}
		prevHashStr := bytesutil.BytesToString(bytesutil.CopyOfRange(rawBytes, pos, pos+size))
		msg.PreviousHash = &prevHashStr
		pos += size
	}

	// Nonce (bytes 20-21: length as short)
	size = int(bytesutil.BytesToShort(bytesutil.CopyOfRange(rawBytes, 20, 22)))
	if size > 0 {
		if pos+size > len(rawBytes) {
			return nil, errors.New("message too short for Nonce field")
		}
		nonceStr := bytesutil.BytesToString(bytesutil.CopyOfRange(rawBytes, pos, pos+size))
		msg.Nonce = &nonceStr
		pos += size
	}

	// DifficultyTarget (byte 22: length 0 or 4)
	size = int(rawBytes[22])
	if size > 0 {
		if pos+size > len(rawBytes) {
			return nil, errors.New("message too short for DifficultyTarget field")
		}
		msg.DifficultyTarget = bytesutil.BytesToInteger(bytesutil.CopyOfRange(rawBytes, pos, pos+size))
		pos += size
	}

	// InfoType (byte 23: length)
	size = int(rawBytes[23])
	if size > 0 {
		if pos+size > len(rawBytes) {
			return nil, errors.New("message too short for InfoType field")
		}
		infoTypeStr := bytesutil.BytesToString(bytesutil.CopyOfRange(rawBytes, pos, pos+size))
		msg.InfoType = &infoTypeStr
		pos += size
	}

	// InfoFormat (byte 24: length)
	size = int(rawBytes[24])
	if size > 0 {
		if pos+size > len(rawBytes) {
			return nil, errors.New("message too short for InfoFormat field")
		}
		infoFormatStr := bytesutil.BytesToString(bytesutil.CopyOfRange(rawBytes, pos, pos+size))
		msg.InfoFormat = &infoFormatStr
		pos += size
	}

	// ContextData (bytes 25-28: length as integer)
	size = bytesutil.BytesToInteger(bytesutil.CopyOfRange(rawBytes, 25, 29))
	if size > 0 {
		if pos+size > len(rawBytes) {
			return nil, errors.New("message too short for ContextData field")
		}
		msg.ContextData = bytesutil.CopyOfRange(rawBytes, pos, pos+size)
		pos += size
	}

	// ContentData (bytes 29-32: length as integer)
	size = bytesutil.BytesToInteger(bytesutil.CopyOfRange(rawBytes, 29, 33))
	if size > 0 {
		if pos+size > len(rawBytes) {
			return nil, errors.New("message too short for ContentData field")
		}
		msg.ContentData = bytesutil.CopyOfRange(rawBytes, pos, pos+size)
	}

	return msg, nil
}

// DecodeBase64 decodes a base64-encoded message
func (m *Message) DecodeBase64(bytes []byte) error {
	decoded := make([]byte, base64.StdEncoding.DecodedLen(len(bytes)))
	n, err := base64.StdEncoding.Decode(decoded, bytes)
	if err != nil {
		return err
	}
	decoded = decoded[:n]

	// Parse binary format
	parsed, err := NewMessageFromBytes(decoded)
	if err != nil {
		return err
	}

	// Copy all fields
	m.ID = parsed.ID
	m.Tag = parsed.Tag
	m.MessageGroupID = parsed.MessageGroupID
	m.SequenceNumber = parsed.SequenceNumber
	m.SequenceTotal = parsed.SequenceTotal
	m.Priority = parsed.Priority
	m.Timestamp = parsed.Timestamp
	m.Publisher = parsed.Publisher
	m.AuthIdentifier = parsed.AuthIdentifier
	m.AuthGroup = parsed.AuthGroup
	m.Version = parsed.Version
	m.ChainPosition = parsed.ChainPosition
	m.Hash = parsed.Hash
	m.PreviousHash = parsed.PreviousHash
	m.Nonce = parsed.Nonce
	m.DifficultyTarget = parsed.DifficultyTarget
	m.InfoType = parsed.InfoType
	m.InfoFormat = parsed.InfoFormat
	m.ContextData = parsed.ContextData
	m.ContentData = parsed.ContentData

	return nil
}

// EncodeBase64 encodes the message to base64
func (m *Message) EncodeBase64() ([]byte, error) {
	bytes, err := m.GetBytes()
	if err != nil {
		return nil, err
	}

	encoded := make([]byte, base64.StdEncoding.EncodedLen(len(bytes)))
	base64.StdEncoding.Encode(encoded, bytes)
	return encoded, nil
}
