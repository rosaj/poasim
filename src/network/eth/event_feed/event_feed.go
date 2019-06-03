package event_feed

import "reflect"

func NewEventFeed() *EventFeed  {
	return &EventFeed{
		feed:	make(map[reflect.Type]func(data interface{}),0),
	}
}

type EventFeed struct {
	feed map[reflect.Type] func(data interface{})
}

func (ef *EventFeed) Subscribe(data interface{}, listener func(data interface{}))  {
	rtyp := reflect.TypeOf(data)
	ef.feed[rtyp] = listener
}
func (ef *EventFeed) SubscribeMultiple(listener func(data interface{}), data ...interface{})  {
	for _, d := range data {
		ef.Subscribe(d, listener)
	}
}

func (ef *EventFeed) Post(data interface{})  {
	rtyp := reflect.TypeOf(data)
	f := ef.feed[rtyp]
	if f != nil {
		f(data)
	}
}


