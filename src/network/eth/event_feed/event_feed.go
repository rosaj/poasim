package event_feed

import "reflect"

func NewEventFeed() *EventFeed  {
	return &EventFeed{
		feed:		make(map[reflect.Type]func(data interface{}),0),
		feeding:	true,
	}
}

type EventFeed struct {
	feed map[reflect.Type] func(data interface{})
	feeding bool
}

func (ef *EventFeed) Subscribe(listener func(data interface{}), data ...interface{})  {
	for _, d := range data {
		rtyp := reflect.TypeOf(d)
		ef.feed[rtyp] = listener
	}
}

func (ef *EventFeed) Post(data interface{})  {
	if ef.feeding {
		rtyp := reflect.TypeOf(data)
		f := ef.feed[rtyp]
		if f != nil {
			f(data)
		}
	}
}

func (ef *EventFeed) Stop()  {
	ef.feeding = false
}

func (ef *EventFeed) Start()  {
	ef.feeding = true
}