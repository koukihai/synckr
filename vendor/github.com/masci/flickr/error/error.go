// Flickr.go error system
package error

// here we define ONLY errors from the library NOT from flickr
// error from flickr have already a code and a message that are returned
// along with the HTTP Response

const (
	ApiError          = 10
	RequestTokenError = 20
	OAuthTokenError   = 30
)

var errors = map[int]string{
	ApiError:          "Flickr API returned an error: ",
	RequestTokenError: "An error occurred during token request: ",
	OAuthTokenError:   "An error occurred while getting the OAuth token: ",
}

type Error struct {
	ErrorCode int
	Message   string
}

// Implement error interface
func (e Error) Error() string {
	return e.Message
}

func NewError(errorCode int, message string) *Error {
	return &Error{
		ErrorCode: errorCode,
		Message:   errors[errorCode] + message,
	}
}
