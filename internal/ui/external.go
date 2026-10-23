package ui

// AddURLsMsg asks the model to queue URLs handed over by `vidl add`, with the
// default format and resolution. When Added is set, it receives the number of
// downloads queued, so it needs room for one value.
type AddURLsMsg struct {
	URLs  []string
	Added chan<- int
}

// ErrorMsg shows an error from outside the UI loop as a toast.
type ErrorMsg struct{ Err error }
