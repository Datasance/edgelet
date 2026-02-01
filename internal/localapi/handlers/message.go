package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/eclipse-iofog/agent-go/internal/messagebus"
	"github.com/eclipse-iofog/agent-go/internal/models"
	"github.com/eclipse-iofog/agent-go/internal/utils/logging"
)

const (
	messageHandlerModuleName = "Message Handler"
)

// MessageHandler handles message API requests
type MessageHandler struct {
	// Will be injected with MessageBus when available
}

// MessageRequest represents a message request
type MessageRequest struct {
	ID string `json:"id"`
}

// MessagesNextResponse represents the response for /v2/messages/next
type MessagesNextResponse struct {
	Status   string                 `json:"status"`
	Count    int                    `json:"count"`
	Messages []map[string]interface{} `json:"messages"`
}

// MessageNewRequest represents a new message request
type MessageNewRequest struct {
	Tag              *string `json:"tag,omitempty"`
	MessageGroupID   *string `json:"groupid,omitempty"`
	SequenceNumber   int     `json:"sequencenumber"`
	SequenceTotal    int     `json:"sequencetotal"`
	Priority         byte    `json:"priority"`
	Publisher        *string `json:"publisher,omitempty"`
	AuthIdentifier   *string `json:"authid,omitempty"`
	AuthGroup        *string `json:"authgroup,omitempty"`
	Version          int16   `json:"version"`
	ChainPosition    int64   `json:"chainposition"`
	Hash             *string `json:"hash,omitempty"`
	PreviousHash     *string `json:"previoushash,omitempty"`
	Nonce            *string `json:"nonce,omitempty"`
	DifficultyTarget int     `json:"difficultytarget"`
	InfoType         *string `json:"infotype,omitempty"`
	InfoFormat       *string `json:"infoformat,omitempty"`
	ContextData      string  `json:"contextdata,omitempty"`
	ContentData      string  `json:"contentdata,omitempty"`
}

// MessageNewResponse represents the response for /v2/messages/new
type MessageNewResponse struct {
	Status    string `json:"status"`
	Timestamp int64  `json:"timestamp"`
	ID        string `json:"id"`
}

// MessagesQueryRequest represents a query request
type MessagesQueryRequest struct {
	ID            string   `json:"id"`
	TimeFrameStart int64  `json:"timeframestart"`
	TimeFrameEnd   int64  `json:"timeframeend"`
	Publishers    []string `json:"publishers"`
}

// MessagesQueryResponse represents the response for /v2/messages/query
type MessagesQueryResponse struct {
	Status        string                   `json:"status"`
	Count         int                      `json:"count"`
	TimeFrameStart int64                   `json:"timeframestart"`
	TimeFrameEnd   int64                   `json:"timeframeend"`
	Messages      []map[string]interface{} `json:"messages"`
}

// HandleMessagesNext handles GET /v2/messages/next
func (h *MessageHandler) HandleMessagesNext(w http.ResponseWriter, r *http.Request) {
	logging.LogDebug(messageHandlerModuleName, "Start Processing messages/next request")

	if r.Method != http.MethodPost {
		logging.LogError(messageHandlerModuleName, "Request method not allowed", nil)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logging.LogError(messageHandlerModuleName, "Failed to read request body", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Parse JSON
	var msgReq MessageRequest
	if err := json.Unmarshal(body, &msgReq); err != nil {
		logging.LogError(messageHandlerModuleName, "Failed to parse JSON", err)
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Get messages from MessageBus
	// The ID in the request is the receiver (container/microservice) ID
	receiverID := msgReq.ID
	if receiverID == "" {
		logging.LogError(messageHandlerModuleName, "Missing receiver ID in request", nil)
		http.Error(w, "Missing receiver ID", http.StatusBadRequest)
		return
	}

	mb := messagebus.GetInstance()
	receiver := mb.GetReceiver(receiverID)
	if receiver == nil {
		// Receiver not found, return empty response
		response := MessagesNextResponse{
			Status:   "okay",
			Count:    0,
			Messages: []map[string]interface{}{},
		}
		jsonData, _ := json.Marshal(response)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(jsonData)
		return
	}

	// Get messages from receiver
	messages, err := receiver.GetMessages()
	if err != nil {
		logging.LogError(messageHandlerModuleName, "Error getting messages", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Convert messages to JSON format
	messageList := make([]map[string]interface{}, 0, len(messages))
	for _, msg := range messages {
		msgMap := make(map[string]interface{})
		if msg.ID != nil {
			msgMap["id"] = *msg.ID
		}
		if msg.Tag != nil {
			msgMap["tag"] = *msg.Tag
		}
		if msg.MessageGroupID != nil {
			msgMap["groupid"] = *msg.MessageGroupID
		}
		msgMap["sequencenumber"] = msg.SequenceNumber
		msgMap["sequencetotal"] = msg.SequenceTotal
		msgMap["priority"] = msg.Priority
		msgMap["timestamp"] = msg.Timestamp
		if msg.Publisher != nil {
			msgMap["publisher"] = *msg.Publisher
		}
		if msg.AuthIdentifier != nil {
			msgMap["authid"] = *msg.AuthIdentifier
		}
		if msg.AuthGroup != nil {
			msgMap["authgroup"] = *msg.AuthGroup
		}
		msgMap["version"] = msg.Version
		msgMap["chainposition"] = msg.ChainPosition
		if msg.Hash != nil {
			msgMap["hash"] = *msg.Hash
		}
		if msg.PreviousHash != nil {
			msgMap["previoushash"] = *msg.PreviousHash
		}
		if msg.Nonce != nil {
			msgMap["nonce"] = *msg.Nonce
		}
		msgMap["difficultytarget"] = msg.DifficultyTarget
		if msg.InfoType != nil {
			msgMap["infotype"] = *msg.InfoType
		}
		if msg.InfoFormat != nil {
			msgMap["infoformat"] = *msg.InfoFormat
		}
		if len(msg.ContextData) > 0 {
			msgMap["contextdata"] = string(msg.ContextData)
		}
		if len(msg.ContentData) > 0 {
			msgMap["contentdata"] = string(msg.ContentData)
		}
		messageList = append(messageList, msgMap)
	}

	response := MessagesNextResponse{
		Status:   "okay",
		Count:    len(messageList),
		Messages: messageList,
	}

	jsonData, err := json.Marshal(response)
	if err != nil {
		logging.LogError(messageHandlerModuleName, "Failed to marshal response", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonData)
	logging.LogDebug(messageHandlerModuleName, "Finished Processing messages/next request")
}

// HandleMessagesNew handles POST /v2/messages/new
func (h *MessageHandler) HandleMessagesNew(w http.ResponseWriter, r *http.Request) {
	logging.LogDebug(messageHandlerModuleName, "Start Processing messages/new request")

	if r.Method != http.MethodPost {
		logging.LogError(messageHandlerModuleName, "Request method not allowed", nil)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logging.LogError(messageHandlerModuleName, "Failed to read request body", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Parse JSON
	var msgReq MessageNewRequest
	if err := json.Unmarshal(body, &msgReq); err != nil {
		logging.LogError(messageHandlerModuleName, "Failed to parse JSON", err)
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Publish message to MessageBus
	mb := messagebus.GetInstance()
	
	// Generate message ID
	messageID := mb.GetNextId()
	if messageID == "" {
		logging.LogError(messageHandlerModuleName, "Failed to generate message ID", nil)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Get timestamp
	timestamp := time.Now().UnixMilli()

	// Create message object
	message := models.NewMessage()
	message.ID = &messageID
	message.Timestamp = timestamp
	if msgReq.Tag != nil {
		message.Tag = msgReq.Tag
	}
	if msgReq.MessageGroupID != nil {
		message.MessageGroupID = msgReq.MessageGroupID
	}
	message.SequenceNumber = msgReq.SequenceNumber
	message.SequenceTotal = msgReq.SequenceTotal
	message.Priority = msgReq.Priority
	if msgReq.Publisher != nil {
		message.Publisher = msgReq.Publisher
	}
	if msgReq.AuthIdentifier != nil {
		message.AuthIdentifier = msgReq.AuthIdentifier
	}
	if msgReq.AuthGroup != nil {
		message.AuthGroup = msgReq.AuthGroup
	}
	message.Version = msgReq.Version
	message.ChainPosition = msgReq.ChainPosition
	if msgReq.Hash != nil {
		message.Hash = msgReq.Hash
	}
	if msgReq.PreviousHash != nil {
		message.PreviousHash = msgReq.PreviousHash
	}
	if msgReq.Nonce != nil {
		message.Nonce = msgReq.Nonce
	}
	message.DifficultyTarget = msgReq.DifficultyTarget
	if msgReq.InfoType != nil {
		message.InfoType = msgReq.InfoType
	}
	if msgReq.InfoFormat != nil {
		message.InfoFormat = msgReq.InfoFormat
	}
	message.ContextData = []byte(msgReq.ContextData)
	message.ContentData = []byte(msgReq.ContentData)

	// Get publisher ID (use publisher from message or default)
	publisherID := ""
	if message.Publisher != nil {
		publisherID = *message.Publisher
	}
	if publisherID == "" {
		logging.LogError(messageHandlerModuleName, "Missing publisher ID", nil)
		http.Error(w, "Missing publisher ID", http.StatusBadRequest)
		return
	}

	// Get publisher and publish
	publisher := mb.GetPublisher(publisherID)
	if publisher == nil {
		logging.LogError(messageHandlerModuleName, "Publisher not found: "+publisherID, nil)
		http.Error(w, "Publisher not found", http.StatusNotFound)
		return
	}

	if err := publisher.Publish(message); err != nil {
		logging.LogError(messageHandlerModuleName, "Error publishing message", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	response := MessageNewResponse{
		Status:    "okay",
		Timestamp: timestamp,
		ID:        messageID,
	}

	jsonData, err := json.Marshal(response)
	if err != nil {
		logging.LogError(messageHandlerModuleName, "Failed to marshal response", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonData)
	logging.LogDebug(messageHandlerModuleName, "Finished Processing messages/new request")
}

// HandleMessagesQuery handles GET /v2/messages/query
func (h *MessageHandler) HandleMessagesQuery(w http.ResponseWriter, r *http.Request) {
	logging.LogDebug(messageHandlerModuleName, "Start Processing messages/query request")

	if r.Method != http.MethodPost {
		logging.LogError(messageHandlerModuleName, "Request method not allowed", nil)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logging.LogError(messageHandlerModuleName, "Failed to read request body", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Parse JSON
	var queryReq MessagesQueryRequest
	if err := json.Unmarshal(body, &queryReq); err != nil {
		logging.LogError(messageHandlerModuleName, "Failed to parse JSON", err)
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Query messages from MessageBus archive
	// The ID in the request is the receiver (container/microservice) ID
	receiverID := queryReq.ID
	if receiverID == "" {
		logging.LogError(messageHandlerModuleName, "Missing receiver ID in query request", nil)
		http.Error(w, "Missing receiver ID", http.StatusBadRequest)
		return
	}

	mb := messagebus.GetInstance()
	
	// Get time frame
	timeFrameStart := queryReq.TimeFrameStart
	timeFrameEnd := queryReq.TimeFrameEnd
	if timeFrameEnd < timeFrameStart {
		logging.LogError(messageHandlerModuleName, "Invalid time frame: end < start", nil)
		http.Error(w, "Invalid time frame", http.StatusBadRequest)
		return
	}

	// Query messages from all specified publishers
	messageList := make([]map[string]interface{}, 0)
	actualTimeFrameEnd := timeFrameEnd

	// If publishers are specified, query each one
	if len(queryReq.Publishers) > 0 {
		for _, publisherID := range queryReq.Publishers {
			// Query messages from this publisher to the receiver
			messages := mb.MessageQuery(publisherID, receiverID, timeFrameStart, timeFrameEnd)
			if messages != nil && len(messages) > 0 {
				// Convert messages to JSON format
				for _, msg := range messages {
					msgMap, err := msg.ToJSON()
					if err != nil {
						logging.LogError(messageHandlerModuleName, "Error converting message to JSON", err)
						continue
					}
					messageList = append(messageList, msgMap)
					// Update actual time frame end with last message timestamp
					if msg.Timestamp > actualTimeFrameEnd {
						actualTimeFrameEnd = msg.Timestamp
					}
				}
			}
		}
	} else {
		// No publishers specified, return empty
		logging.LogWarn(messageHandlerModuleName, "No publishers specified in query request")
	}

	response := MessagesQueryResponse{
		Status:         "okay",
		Count:          len(messageList),
		TimeFrameStart: timeFrameStart,
		TimeFrameEnd:   actualTimeFrameEnd,
		Messages:       messageList,
	}

	jsonData, err := json.Marshal(response)
	if err != nil {
		logging.LogError(messageHandlerModuleName, "Failed to marshal response", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonData)
	logging.LogDebug(messageHandlerModuleName, "Finished Processing messages/query request")
}
