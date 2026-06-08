package logs

const DefaultTail = "100"

// Options carries log query flags shared by system and microservice logs commands.
type Options struct {
	Follow     bool
	Tail       string
	Since      string
	Until      string
	Timestamps bool
}
