package consoleChan

import ()

type Client interface {
	Dial() (Session, error)
}
