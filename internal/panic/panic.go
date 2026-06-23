package panic

import (
	"fmt"
	"os"
	"runtime"
)

type Handler struct {
	logFunc func(string, ...interface{})
	appDir  string
}

func NewHandler(logFunc func(string, ...interface{}), appDir string) *Handler {
	return &Handler{logFunc: logFunc, appDir: appDir}
}

func (h *Handler) Setup() {
	go h.recoverLoop()
}

func (h *Handler) recoverLoop() {
	if r := recover(); r != nil {
		h.handle(r)
	}
}

func Safe(fn func()) {
	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 4096)
			n := runtime.Stack(buf, false)
			fmt.Fprintf(os.Stderr, "PANIC: %v\nstack:\n%s\n", r, string(buf[:n]))
		}
	}()
	fn()
}

func (h *Handler) handle(r interface{}) {
	buf := make([]byte, 8192)
	n := runtime.Stack(buf, false)

	msg := fmt.Sprintf("%v", r)
	stack := string(buf[:n])

	if h.logFunc != nil {
		h.logFunc("critical_error", "panic", msg, "stack", stack)
	}

	fmt.Fprintf(os.Stderr, "\n═══════════════════════════════════════════\n")
	fmt.Fprintf(os.Stderr, "  ПРОИЗОШЛА КРИТИЧЕСКАЯ ОШИБКА\n")
	fmt.Fprintf(os.Stderr, "═══════════════════════════════════════════\n")
	fmt.Fprintf(os.Stderr, "  %s\n\n", msg)
	fmt.Fprintf(os.Stderr, "  Пожалуйста, отправьте файл launcher.log\n")
	fmt.Fprintf(os.Stderr, "  разработчику для исправления проблемы.\n")
	fmt.Fprintf(os.Stderr, "═══════════════════════════════════════════\n\n")
}
