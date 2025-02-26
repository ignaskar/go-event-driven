package http

import (
	"github.com/ThreeDotsLabs/watermill/message"
)

type Handler struct {
	publisher message.Publisher
}
