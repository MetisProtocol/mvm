package cmd

import (
	"io"
	"log/slog"

	"github.com/MetisProtocol/mvm/l2geth/log"
)

func Logger(w io.Writer, lvl slog.Level, ctx ...interface{}) log.Logger {
	logger := log.New(ctx)
	//if term.IsTerminal(int(os.Stdout.Fd())) {
	//	logger.SetHandler(log.LvlFilterHandler(log.Lvl(lvl), log.StreamHandler(w, log.TerminalFormat(true))))
	//} else {
	//	logger.SetHandler(log.LvlFilterHandler(log.Lvl(lvl), log.StreamHandler(w, log.LogfmtFormat())))
	//}
	logger.SetHandler(log.StreamHandler(w, log.LogfmtFormat()))

	return logger
}

// rawLogHandler returns a handler that strips out the time attribute
func rawLogHandler(wr io.Writer, lvl slog.Level) slog.Handler {
	return slog.NewTextHandler(wr, &slog.HandlerOptions{
		ReplaceAttr: replaceAttr,
		Level:       &leveler{lvl},
	})
}

type leveler struct{ minLevel slog.Level }

func (l *leveler) Level() slog.Level {
	return l.minLevel
}

func replaceAttr(_ []string, attr slog.Attr) slog.Attr {
	if attr.Key == slog.TimeKey {
		return slog.Attr{}
	}
	return attr
}
