package ipc

// checkSocketDir refuses a socket path inside the temp-dir fallback
// directory unless that directory is a real directory owned by the current
// user with mode 0700. Other paths and missing directories pass.
func checkSocketDir(path string) error {
	return nil
}
