package http

import (
	"github.com/ThreeDotsLabs/watermill/components/cqrs"
)

type Handler struct {
	eventBus *cqrs.EventBus
}
