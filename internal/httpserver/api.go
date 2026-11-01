package httpserver

import "schooldiarysvu/internal/service"

type API struct {
	service      *service.Service
	cookieSecure bool
	trustProxy   bool
}
