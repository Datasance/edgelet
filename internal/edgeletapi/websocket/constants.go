package websocket

// WebSocket opcodes
const (
	OpcodePing           byte = 0x9
	OpcodePong           byte = 0xA
	OpcodeACK            byte = 0xB
	OpcodeControlSignal  byte = 0xC
	OpcodeMSG            byte = 0xD
	OpcodeReceipt        byte = 0xE
	OpcodeResourceSignal byte = 0xF
)
