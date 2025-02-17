package event

import (
	"fmt"
	"sync"
	"github.com/wailsapp/wails/lib/logger"
	"github.com/wailsapp/wails/lib/messages"
	"github.com/wailsapp/wails/lib/interfaces"
)

// Manager handles and processes events
type Manager struct {
	incomingEvents chan *messages.EventData
	listeners      map[string][]*eventListener
	exit           bool
	log            *logger.CustomLogger
	renderer       interfaces.Renderer // Messages will be dispatched to the frontend
}

// NewManager creates a new event manager with a 100 event buffer
func NewManager() interfaces.EventManager {
	return &Manager{
		incomingEvents: make(chan *messages.EventData, 100),
		listeners:      make(map[string][]*eventListener),
		exit:           false,
		log:            logger.NewCustomLogger("Events"),
	}
}

// PushEvent places the given event on to the event queue
func (e *Manager) PushEvent(eventData *messages.EventData) {
	e.incomingEvents <- eventData
}

// eventListener holds a callback function which is invoked when
// the event listened for is emitted. It has a counter which indicates
// how the total number of events it is interested in. A value of zero
// means it does not expire (default).
type eventListener struct {
	callback func(...interface{}) // Function to call with emitted event data
	counter  int                  // Expire after counter callbacks. 0 = infinite
	expired  bool                 // Indicates if the listener has expired
}

// Creates a new event listener from the given callback function
func (e *Manager) addEventListener(eventName string, callback func(...interface{}), counter int) error {

	// Sanity check inputs
	if callback == nil {
		return fmt.Errorf("nil callback bassed to addEventListener")
	}

	// Check event has been registered before
	if e.listeners[eventName] == nil {
		e.listeners[eventName] = []*eventListener{}
	}

	// Create the callback
	listener := &eventListener{
		callback: callback,
		counter:  counter,
	}

	// Register listener
	e.listeners[eventName] = append(e.listeners[eventName], listener)

	// All good mate
	return nil
}

// On adds a listener for the given event
func (e *Manager) On(eventName string, callback func(...interface{})) {
	// Add a persistent eventListener (counter = 0)
	e.addEventListener(eventName, callback, 0)
}

// Emit broadcasts the given event to the subscribed listeners
func (e *Manager) Emit(eventName string, optionalData ...interface{}) {
	e.incomingEvents <- &messages.EventData{Name: eventName, Data: optionalData}
}

// Start the event manager's queue processing
func (e *Manager) Start(renderer interfaces.Renderer) {

	e.log.Info("Starting")

	// Store renderer
	e.renderer = renderer

	// Set up waitgroup so we can wait for goroutine to start
	var wg sync.WaitGroup
	wg.Add(1)

	// Run main loop in separate goroutine
	go func() {
		wg.Done()
		e.log.Info("Listening")
		for e.exit == false {
			// TODO: Listen for application exit
			select {
			case event := <-e.incomingEvents:
				e.log.DebugFields("Got Event", logger.Fields{
					"data": event.Data,
					"name": event.Name,
				})

				// Notify renderer
				e.renderer.NotifyEvent(event)

				// Notify Go listeners
				var listenersToRemove []*eventListener

				// Iterate listeners
				for _, listener := range e.listeners[event.Name] {

					// Call listener, perhaps with data
					if event.Data == nil {
						go listener.callback()
					} else {
						unpacked := event.Data.([]interface{})
						go listener.callback(unpacked...)
					}

					// Update listen counter
					if listener.counter > 0 {
						listener.counter = listener.counter - 1
						if listener.counter == 0 {
							listener.expired = true
						}
					}
				}

				// Remove expired listeners in place
				if len(listenersToRemove) > 0 {
					listeners := e.listeners[event.Name][:0]
					for _, listener := range listeners {
						if !listener.expired {
							listeners = append(listeners, listener)
						}
					}
				}
			}
		}
	}()

	// Wait for goroutine to start
	wg.Wait()
}

func (e *Manager) stop() {
	e.exit = true
}
