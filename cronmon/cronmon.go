package cronmon

// Journaler describes an event logger.
type Journaler interface {
	Write(Event) error
}
