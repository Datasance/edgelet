package websocket

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/datasance/edgelet/internal/auth"
	"github.com/datasance/edgelet/internal/utils/logging"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
)

const (
	controlHandlerModuleName = "Control WebSocket Handler"
	requestIDHeader          = "X-Request-Id"
	v3ControlPath            = "/v1/microservices/control"
)

var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			// Check origin against configured allowed origins
			// For now, allow localhost and same-origin requests
			origin := r.Header.Get("Origin")
			if origin == "" {
				// No origin header, allow (same-origin or non-browser client)
				return true
			}

			// Allow localhost origins
			if strings.Contains(origin, "localhost") || strings.Contains(origin, "127.0.0.1") {
				return true
			}

			// In production, you might want to check against a whitelist
			// For now, allow all origins (can be restricted later)
			return true
		},
	}
	validateLocalJWTFn = auth.ValidateEdgeletAPIJWT
	authorizeV3WSFn    = authorizeV3WebsocketClaims
)

// ControlHandler handles control WebSocket connections
type ControlHandler struct {
	manager *Manager
}

// NewControlHandler creates a new control WebSocket handler
func NewControlHandler() *ControlHandler {
	return &ControlHandler{
		manager: GetManager(),
	}
}

// Handle handles the WebSocket upgrade and connection
func (h *ControlHandler) Handle(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	requestID := strings.TrimSpace(r.Header.Get(requestIDHeader))
	if requestID == "" {
		requestID = generateUUIDv4()
	}
	w.Header().Set(requestIDHeader, requestID)

	if r.Method != http.MethodGet {
		emitWSReject(r, requestID, "method_not_allowed", http.StatusMethodNotAllowed, nil)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	isV3 := r.URL.Path == v3ControlPath
	if !isV3 {
		emitWSReject(r, requestID, "rbac_unmapped_route", http.StatusNotFound, nil)
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		emitWSReject(r, requestID, "missing_bearer_prefix", http.StatusUnauthorized, nil)
		http.Error(w, "Missing bearer token", http.StatusUnauthorized)
		return
	}
	token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
	if token == "" {
		emitWSReject(r, requestID, "empty_token", http.StatusUnauthorized, nil)
		http.Error(w, "Missing bearer token", http.StatusUnauthorized)
		return
	}
	tokenMeta := parseTokenMeta(token)
	validationResult, err := validateLocalJWTFn(token)
	if err != nil {
		emitWSReject(r, requestID, "websocket_unauthorized", http.StatusUnauthorized, tokenMeta)
		http.Error(w, "Invalid JWT token", http.StatusUnauthorized)
		return
	}
	tokenMeta = mergeTokenMeta(tokenMeta, tokenMetaFromClaims(validationResult.Claims))

	id := tokenMeta["microserviceUUID"]
	if strings.TrimSpace(id) == "" {
		emitWSReject(r, requestID, "websocket_unauthorized", http.StatusForbidden, tokenMeta)
		http.Error(w, "microservice identity claim is required", http.StatusForbidden)
		return
	}
	if !authorizeV3WSFn(validationResult.Claims) {
		emitWSReject(r, requestID, "rbac_denied", http.StatusForbidden, tokenMeta)
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// Upgrade to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logging.LogError(controlHandlerModuleName, "Failed to upgrade connection", err)
		emitWSReject(r, requestID, "websocket_upgrade_failed", http.StatusBadRequest, tokenMeta)
		return
	}

	// Register connection
	connection := h.manager.AddConnection(ControlWebSocket, id, conn)
	logging.LogDebug(controlHandlerModuleName, "Websocket for the real-time control signals is open")
	emitWSEvent("info", map[string]interface{}{
		"event":            "edgeletapi.access",
		"requestId":        requestID,
		"path":             r.URL.Path,
		"method":           r.Method,
		"route":            r.URL.Path,
		"transport":        "tcp",
		"scheme":           wsScheme(r),
		"status":           http.StatusSwitchingProtocols,
		"durationMs":       time.Since(start).Milliseconds(),
		"sub":              tokenMeta["sub"],
		"tokenUse":         tokenMeta["tokenUse"],
		"jtiHash":          tokenMeta["jtiHash"],
		"microserviceUuid": id,
	})

	// Handle connection in goroutine
	go h.handleConnection(connection)
}

func emitWSReject(r *http.Request, requestID, reasonCode string, status int, tokenMeta map[string]string) {
	fields := map[string]interface{}{
		"event":      "edgeletapi.reject",
		"requestId":  requestID,
		"reasonCode": reasonCode,
		"path":       r.URL.Path,
		"method":     r.Method,
		"transport":  "tcp",
		"scheme":     wsScheme(r),
		"status":     status,
	}
	if tokenMeta != nil {
		if v := tokenMeta["sub"]; v != "" {
			fields["sub"] = v
		}
		if v := tokenMeta["tokenUse"]; v != "" {
			fields["tokenUse"] = v
		}
		if v := tokenMeta["jtiHash"]; v != "" {
			fields["jtiHash"] = v
		}
	}
	emitWSEvent("warn", fields)
}

func emitWSEvent(level string, fields map[string]interface{}) {
	wsLogSink(level, fields)
}

var wsLogSink = func(level string, fields map[string]interface{}) {
	payload, _ := json.Marshal(fields)
	switch strings.ToLower(level) {
	case "warn":
		logging.LogWarn(controlHandlerModuleName, string(payload))
	case "error":
		logging.LogError(controlHandlerModuleName, string(payload), nil)
	default:
		logging.LogInfo(controlHandlerModuleName, string(payload))
	}
}

func wsScheme(r *http.Request) string {
	if r.TLS != nil {
		return "wss"
	}
	return "ws"
}

func parseTokenMeta(token string) map[string]string {
	meta := map[string]string{}
	parsed, _, err := jwt.NewParser().ParseUnverified(token, jwt.MapClaims{})
	if err != nil {
		return meta
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return meta
	}
	if sub, _ := claims["sub"].(string); strings.TrimSpace(sub) != "" {
		meta["sub"] = sub
	}
	if tokenUse, _ := claims["tokenUse"].(string); strings.TrimSpace(tokenUse) != "" {
		meta["tokenUse"] = tokenUse
	}
	if jti, _ := claims["jti"].(string); strings.TrimSpace(jti) != "" {
		sum := sha256.Sum256([]byte(strings.TrimSpace(jti)))
		meta["jtiHash"] = hex.EncodeToString(sum[:])
	}
	if iofog, ok := claims["edgelet.iofog.org"].(map[string]interface{}); ok {
		if microservice, ok := iofog["microservice"].(map[string]interface{}); ok {
			if uuid, _ := microservice["uuid"].(string); strings.TrimSpace(uuid) != "" {
				meta["microserviceUUID"] = strings.TrimSpace(uuid)
			}
		}
	}
	return meta
}

func tokenMetaFromClaims(claims jwt.MapClaims) map[string]string {
	meta := map[string]string{}
	if sub, _ := claims["sub"].(string); strings.TrimSpace(sub) != "" {
		meta["sub"] = sub
	}
	if tokenUse, _ := claims["tokenUse"].(string); strings.TrimSpace(tokenUse) != "" {
		meta["tokenUse"] = tokenUse
	}
	if jti, _ := claims["jti"].(string); strings.TrimSpace(jti) != "" {
		sum := sha256.Sum256([]byte(strings.TrimSpace(jti)))
		meta["jtiHash"] = hex.EncodeToString(sum[:])
	}
	if iofog, ok := claims["edgelet.iofog.org"].(map[string]interface{}); ok {
		if microservice, ok := iofog["microservice"].(map[string]interface{}); ok {
			if uuid, _ := microservice["uuid"].(string); strings.TrimSpace(uuid) != "" {
				meta["microserviceUUID"] = strings.TrimSpace(uuid)
			}
		}
	}
	return meta
}

func mergeTokenMeta(a, b map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range a {
		if strings.TrimSpace(v) != "" {
			out[k] = v
		}
	}
	for k, v := range b {
		if strings.TrimSpace(v) != "" {
			out[k] = v
		}
	}
	return out
}

func authorizeV3WebsocketClaims(claims jwt.MapClaims) bool {
	tokenUse, _ := claims["tokenUse"].(string)
	sub, _ := claims["sub"].(string)
	if strings.TrimSpace(tokenUse) == "edgeletapi" && (strings.HasPrefix(sub, "system:edgeletadmin:") || sub == "system:edgeletadmin:bootstrap") {
		return true
	}

	iofogRaw, ok := claims["edgelet.iofog.org"].(map[string]interface{})
	if !ok {
		return false
	}
	rbacRaw, ok := iofogRaw["rbac"].(map[string]interface{})
	if !ok {
		return false
	}
	rulesRaw, ok := rbacRaw["rulesByGroup"].(map[string]interface{})
	if !ok {
		return false
	}

	apiGroups := []string{"edgelet.iofog.org/v1", "edgelet.iofog.org/v1"}
	for _, group := range apiGroups {
		if groupRulesMatch(rulesRaw, group, "microservices/control/self", "get") {
			return true
		}
	}
	return false
}

func groupRulesMatch(groups map[string]interface{}, group, resource, verb string) bool {
	rulesRaw, exists := groups[group]
	if !exists {
		rulesRaw, exists = groups["*"]
		if !exists {
			return false
		}
	}

	rulesSlice, ok := rulesRaw.([]interface{})
	if !ok {
		return false
	}
	for _, ruleRaw := range rulesSlice {
		rule, ok := ruleRaw.(map[string]interface{})
		if !ok {
			continue
		}
		if !stringListMatch(rule["resources"], resource) {
			continue
		}
		if !stringListMatch(rule["verbs"], verb) {
			continue
		}
		return true
	}
	return false
}

func stringListMatch(raw interface{}, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	switch v := raw.(type) {
	case []interface{}:
		values := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				values = append(values, strings.TrimSpace(s))
			}
		}
		return slices.Contains(values, "*") || slices.Contains(values, target)
	case []string:
		values := make([]string, 0, len(v))
		for _, item := range v {
			values = append(values, strings.TrimSpace(item))
		}
		return slices.Contains(values, "*") || slices.Contains(values, target)
	default:
		return false
	}
}

func generateUUIDv4() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "00000000-0000-4000-8000-000000000000"
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return hex.EncodeToString(b[0:4]) + "-" +
		hex.EncodeToString(b[4:6]) + "-" +
		hex.EncodeToString(b[6:8]) + "-" +
		hex.EncodeToString(b[8:10]) + "-" +
		hex.EncodeToString(b[10:16])
}

// handleConnection handles messages from a WebSocket connection
func (h *ControlHandler) handleConnection(conn *Connection) {
	defer func() {
		if r := recover(); r != nil {
			logging.LogError(controlHandlerModuleName, "Panic recovered", fmt.Errorf("%v", r))
		}
	}()
	defer func() {
		if err := conn.Conn.Close(); err != nil {
			logging.LogWarn(controlHandlerModuleName, "Failed to close WebSocket connection: "+err.Error())
		}
		h.manager.RemoveConnection(ControlWebSocket, conn.ID)
		logging.LogDebug(controlHandlerModuleName, "Control WebSocket connection closed")
	}()

	for {
		messageType, message, err := conn.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				logging.LogError(controlHandlerModuleName, "WebSocket error", err)
			}
			break
		}

		if messageType == websocket.BinaryMessage {
			h.handleBinaryMessage(conn, message)
		} else if messageType == websocket.PingMessage {
			h.handlePing(conn)
		} else if messageType == websocket.CloseMessage {
			break
		}
	}
}

// handleBinaryMessage handles binary WebSocket messages
func (h *ControlHandler) handleBinaryMessage(conn *Connection, data []byte) {
	if len(data) < 1 {
		return
	}

	opcode := data[0]

	switch opcode {
	case OpcodePing:
		// Respond with pong
		pongData := []byte{OpcodePong}
		if err := conn.Conn.WriteMessage(websocket.BinaryMessage, pongData); err != nil {
			logging.LogError(controlHandlerModuleName, "Failed to send pong", err)
		}
	case OpcodeACK:
		// Acknowledge received
		logging.LogDebug(controlHandlerModuleName, "ACK received")
	default:
		logging.LogDebug(controlHandlerModuleName, "Unknown opcode")
	}
}

// handlePing handles ping messages
func (h *ControlHandler) handlePing(conn *Connection) {
	// Respond with pong
	pongData := []byte{OpcodePong}
	if err := conn.Conn.WriteMessage(websocket.PongMessage, pongData); err != nil {
		logging.LogError(controlHandlerModuleName, "Failed to send pong", err)
	}
}

// SendControlSignal sends a control signal to a specific container
func (h *ControlHandler) SendControlSignal(containerID string) error {
	conn, exists := h.manager.GetConnection(ControlWebSocket, containerID)
	if !exists {
		return nil // Connection doesn't exist, nothing to do
	}

	signalData := []byte{OpcodeControlSignal}
	return conn.Conn.WriteMessage(websocket.BinaryMessage, signalData)
}

// SendControlSignalToAll sends a control signal to all connected containers
func (h *ControlHandler) SendControlSignalToAll(changedConfigIDs []string) {
	allConnections := h.manager.GetAllConnections(ControlWebSocket)

	for _, containerID := range changedConfigIDs {
		if conn, exists := allConnections[containerID]; exists {
			signalData := []byte{OpcodeControlSignal}
			if err := conn.Conn.WriteMessage(websocket.BinaryMessage, signalData); err != nil {
				logging.LogError(controlHandlerModuleName, "Failed to send control signal", err)
			}
		}
	}
}

// SendResourceSignal sends a resource signal to all connected containers
func (h *ControlHandler) SendResourceSignal() {
	allConnections := h.manager.GetAllConnections(ControlWebSocket)

	for _, conn := range allConnections {
		signalData := []byte{OpcodeResourceSignal}
		if err := conn.Conn.WriteMessage(websocket.BinaryMessage, signalData); err != nil {
			logging.LogError(controlHandlerModuleName, "Failed to send resource signal", err)
		}
	}
}
