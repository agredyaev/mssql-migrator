package types

type ErrorNotifier struct{ fn func(msg string) }

func (n *ErrorNotifier) SetErrorHandler(fn func(msg string)) { n.fn = fn }

func (n *ErrorNotifier) Notify(msg string) {
	if n.fn != nil {
		n.fn(msg)
	}
}
