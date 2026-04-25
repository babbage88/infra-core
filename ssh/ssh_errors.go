package ssh

type SshInitializationError struct {
	Message string `json:"message"`
	Code    int    `json:"-"`
	Err     error  `json:"-"`
}

func SshErrorWrapper(code int, err error, message string) error {
	return SshInitializationError{
		Message: message,
		Code:    code,
		Err:     err,
	}
}

func (err SshInitializationError) Unwrap() error { return err.Err }

func (err SshInitializationError) Error() string {
	if err.Err != nil {
		return err.Err.Error()
	}
	return err.Message
}

type SftpInitializationError struct {
	Message string `json:"message"`
	Code    int    `json:"-"`
	Err     error  `json:"-"`
}

func SftpInitErrorWrapper(code int, err error, message string) error {
	return SftpInitializationError{
		Message: message,
		Code:    code,
		Err:     err,
	}
}

func (err SftpInitializationError) Unwrap() error { return err.Err }

func (err SftpInitializationError) Error() string {
	if err.Err != nil {
		return err.Err.Error()
	}
	return err.Message
}

type SftpFileCreationializationError struct {
	Message string `json:"message"`
	Code    int    `json:"-"`
	Err     error  `json:"-"`
}

func SftpFileCreationErrorWrapper(code int, err error, message string) error {
	return SftpFileCreationializationError{
		Message: message,
		Code:    code,
		Err:     err,
	}
}

func (err SftpFileCreationializationError) Unwrap() error { return err.Err }

func (err SftpFileCreationializationError) Error() string {
	if err.Err != nil {
		return err.Err.Error()
	}
	return err.Message
}

type SftpTransferError struct {
	Message string `json:"message"`
	Code    int    `json:"-"`
	Err     error  `json:"-"`
}

func SftpErrorWrapper(code int, err error, message string) error {
	return SftpTransferError{
		Message: message,
		Code:    code,
		Err:     err,
	}
}

func (err SftpTransferError) Unwrap() error { return err.Err }

func (err SftpTransferError) Error() string {
	if err.Err != nil {
		return err.Err.Error()
	}
	return err.Message
}
