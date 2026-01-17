package services

import "github.com/DmytroBuzhylov/echofog-core/internal/dispatcher"

type Service interface {
	dispatcher.Handler
	GetSubscribedTypes() []interface{}
}
