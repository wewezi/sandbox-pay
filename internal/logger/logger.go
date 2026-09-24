// Package logger 带上下文名的极简 logger：New("PaymentService") + .Log/.Warn/.Error/.Debug。
package logger

import (
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

var levelWidth = 5

// silent 置为 true 时只关输出、不改行为。
// 测试里会故意走失败分支（通知重试耗尽、退款状态不对），
// 这些 WARN/ERROR 混进 go test 的输出里会让一次全绿的运行看起来像炸了。
var silent atomic.Bool

func init() {
	silent.Store(os.Getenv("LOG_SILENT") == "1")
}

// SetSilent 关掉/打开输出，测试用。
func SetSilent(v bool) { silent.Store(v) }

// Logger 每个组件一个，context 进日志行。
type Logger struct {
	context string
}

// New 创建一个带 context 名的 logger。
func New(context string) *Logger { return &Logger{context: context} }

// 时间戳和原 TS 版对齐：ISO 去掉 T 和 Z，毫秒保留。
func stamp() string {
	return time.Now().UTC().Format("2006-01-02 15:04:05.000")
}

func pad(level string) string {
	if len(level) >= levelWidth {
		return level
	}
	return level + strings.Repeat(" ", levelWidth-len(level))
}

func (l *Logger) write(level string, stderr bool, message string, extra string) {
	if silent.Load() {
		return
	}
	line := fmt.Sprintf("%s %s [%s] %s\n", stamp(), pad(level), l.context, message)
	if extra != "" {
		line += extra + "\n"
	}
	if stderr {
		os.Stderr.WriteString(line)
	} else {
		os.Stdout.WriteString(line)
	}
}

func (l *Logger) Log(message string)  { l.write("LOG", false, message, "") }
func (l *Logger) Warn(message string) { l.write("WARN", true, message, "") }
func (l *Logger) Debug(message string) {
	if os.Getenv("DEBUG") != "" {
		l.write("DEBUG", false, message, "")
	}
}

// Error 第二个参数是 stack（可选），原样跟在行后面。
func (l *Logger) Error(message string, stack ...string) {
	extra := ""
	if len(stack) > 0 {
		extra = stack[0]
	}
	l.write("ERROR", true, message, extra)
}
