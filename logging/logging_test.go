package logging

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// testLogger captures the log output from slog and logs it via the testing.T object,
// resulting in log lines only written if test is run with verbose
type testLogger struct {
	t *testing.T
}

func (tl *testLogger) Write(data []byte) (n int, err error) {
	tl.t.Log(strings.TrimRight(string(data), string('\n')))
	return len(data), nil
}

func TestInitialization(t *testing.T) {
	t.Run("unknown level", func(t *testing.T) {
		_, err := Init(LevelFromString("kittens"), EncodingJSON)
		require.NotNil(t, err)
	})

	t.Run("unknown encoding", func(t *testing.T) {
		_, err := Init(LevelDebug, Encoding("windings"))
		require.NotNil(t, err)
	})
}

func TestFileOutputOption(t *testing.T) {
	var tests = []struct {
		in            string
		expectedError error
		clean         bool
	}{
		{"stdout", nil, false},
		{"stderr", nil, false},
		{"devnull", nil, false},
		{"", errEmptyFilePath, false},
		{"tmpfile", nil, true},
	}

	for i, test := range tests {
		test := test
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			_, shutdown, err := New(LevelDebug, EncodingLogfmt, WithFileOutput(test.in))
			require.ErrorIs(t, test.expectedError, err)
			if shutdown != nil {
				_ = shutdown()
			}
			if test.clean {
				require.Nil(t, os.RemoveAll(test.in))
			}
		})
	}
}

func TestFileOutputShutdown(t *testing.T) {
	tmpFile := t.TempDir() + "/test.log"
	_, shutdown, err := New(LevelDebug, EncodingLogfmt, WithFileOutput(tmpFile))
	require.Nil(t, err)
	require.NotNil(t, shutdown)

	// shutdown should close the file without error
	require.Nil(t, shutdown())
}

func TestBogusInput(t *testing.T) {
	_, _, err := NewFromContext(context.Background(), 42, Encoding("00000000000llllllllll"))
	require.NotNil(t, err)
}

func TestNewPlainLogger(t *testing.T) {
	buf := &bytes.Buffer{}
	buf2 := &bytes.Buffer{}

	logger, _, err := NewFromContext(context.Background(), LevelInfo, EncodingPlain,
		WithOutput(buf),
		WithErrorOutput(buf2),
	)
	require.Nil(t, err)

	// plain handler now prints attributes
	logger.WithGroup("grp").With(slog.String("else", "matters")).Info("hello world")
	require.Contains(t, buf.String(), "Hello world")
	require.Contains(t, buf.String(), "else=matters")

	buf.Reset()

	logger.WithGroup("grp").With(slog.String("else", "matters")).Error("hello error")
	require.Contains(t, buf2.String(), "Hello error")
	require.Contains(t, buf2.String(), "else=matters")

	buf.Reset()

	logger.Info("")
	require.Equal(t, "\n", buf.String())

	buf.Reset()

	logger.Debug("i shouldn't show up")
	require.Equal(t, "", buf.String())
}

func TestPlainHandlerWithAttrs(t *testing.T) {
	buf := &bytes.Buffer{}
	logger, _, err := New(LevelInfo, EncodingPlain, WithOutput(buf))
	require.Nil(t, err)

	logger.With("key", "val").Info("test message")
	require.Contains(t, buf.String(), "key=val")
}

func TestLogConcurrent(t *testing.T) {
	_, err := Init(LevelFromString("debug"), EncodingLogfmt, WithOutput(&testLogger{t}))
	if err != nil {
		t.Fatalf(err.Error())
	}

	t.Run("samectxhierarchy", func(t *testing.T) {
		ctx := WithFields(context.Background(), slog.String("hello", "world"))
		numConcurrent := 32

		logger := FromContext(ctx)
		logger.Info("before go-routines")

		var wg sync.WaitGroup
		wg.Add(numConcurrent)
		for i := 0; i < numConcurrent; i++ {
			go func(n int, ctx context.Context) {
				defer wg.Done()

				f1ctx := WithFields(ctx, slog.Int("fval", n))
				l2 := FromContext(f1ctx)
				l2.Infof("f%d", n)
			}(i, ctx)
		}
		wg.Wait()

		logger = FromContext(ctx)
		logger.Infof("after %d go-routines", numConcurrent)
	})
}

type mockExiter struct {
	t *testing.T
}

func (m mockExiter) Exit(code int) {
	m.t.Logf("mocking exit with code: %d", code)
}

type mockPanicker struct {
	t *testing.T
}

func (m mockPanicker) Panic(msg string) {
	m.t.Logf("mocking panic with message: %s", msg)
}

func TestCaller(t *testing.T) {
	_, err := Init(LevelFromString("warn"), EncodingLogfmt,
		WithOutput(&testLogger{t}),
		WithCaller(true),
		WithName("testing"),
		WithVersion("snapshot"),
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	logger := Logger()
	logger.Error(errors.New("testing plain error caller"))

	// shouldn't show up
	logger.Info("the things you do for love")
}

var testMsg = "log me like one of your French girls"

// lineCounterOutput is a way to check how many log lines were received
// on the output
type lineCounterOutput struct {
	lines int
}

// Write implements the io.Writer interface
func (l *lineCounterOutput) Write(data []byte) (n int, err error) {
	l.lines++
	return len(data), nil
}

func TestLevelSplitHandler(t *testing.T) {
	t.Run("check split outputs", func(t *testing.T) {
		lcostd := &lineCounterOutput{}
		lcoerrs := &lineCounterOutput{}

		_, err := Init(LevelFromString("debug"), EncodingJSON,
			WithOutput(lcostd),
			WithErrorOutput(lcoerrs),
		)
		require.Nil(t, err)

		logger := Logger()
		logger.Info("a message")
		logger.Warn("a warning message")
		logger.Error("an error")

		require.Equal(t, 2, lcostd.lines)
		require.Equal(t, 1, lcoerrs.lines)
	})

	t.Run("check split output visually", func(t *testing.T) {
		// re-initialize so messages can be visually inspected with -v
		_, err := Init(LevelFromString("debug"), EncodingLogfmt,
			WithName("test"),
			WithVersion("snapshot"),
			// tests the WithGroup/WithAttr from the callerHandler
			WithCaller(true),
			WithOutput(os.Stdout),
			WithErrorOutput(os.Stderr),
		)
		require.Nil(t, err)

		logger := Logger().WithGroup("bubu").With("hello", "kitty")
		logger.Info("a message")
		logger.Error("an error")
	})
}

func TestDebug(t *testing.T) {
	lco := &lineCounterOutput{}

	_, err := Init(LevelFromString("debug"), EncodingJSON, WithOutput(lco))
	require.Nil(t, err)

	logger := Logger()
	logger.Debug(testMsg)
	logger.Debugf("%s", testMsg)

	require.Equal(t, 2, lco.lines)
}

func TestInfo(t *testing.T) {
	lco := &lineCounterOutput{}

	_, err := Init(LevelFromString("info"), EncodingJSON, WithOutput(lco))
	require.Nil(t, err)

	logger := Logger()
	logger.Debug(testMsg)
	logger.Debugf("%s", testMsg)
	logger.Info(testMsg)
	logger.Infof("%s", testMsg)

	require.Equal(t, 2, lco.lines)
}

func TestWarn(t *testing.T) {
	lco := &lineCounterOutput{}

	_, err := Init(LevelFromString("warn"), EncodingJSON, WithOutput(lco))
	require.Nil(t, err)

	logger := Logger()
	logger.Debug(testMsg)
	logger.Debugf("%s", testMsg)
	logger.Info(testMsg)
	logger.Infof("%s", testMsg)
	logger.Warn(testMsg)
	logger.Warnf("%s", testMsg)

	require.Equal(t, 2, lco.lines)
}

func TestError(t *testing.T) {
	lco := &lineCounterOutput{}

	_, err := Init(LevelFromString("error"), EncodingJSON, WithOutput(lco))
	require.Nil(t, err)

	logger := Logger()
	logger.Debug(testMsg)
	logger.Debugf("%s", testMsg)
	logger.Info(testMsg)
	logger.Infof("%s", testMsg)
	logger.Warn(testMsg)
	logger.Warnf("%s", testMsg)
	logger.Error(testMsg)
	logger.Errorf("%s", testMsg)

	require.Equal(t, 2, lco.lines)
}

func TestFatal(t *testing.T) {
	lco := &lineCounterOutput{}

	_, err := Init(LevelFromString("fatal"), EncodingJSON, WithOutput(lco))
	require.Nil(t, err)

	logger := Logger().exiter(mockExiter{t})
	logger.Debug(testMsg)
	logger.Debugf("%s", testMsg)
	logger.Info(testMsg)
	logger.Infof("%s", testMsg)
	logger.Warn(testMsg)
	logger.Warnf("%s", testMsg)
	logger.Error(testMsg)
	logger.Errorf("%s", testMsg)
	logger.Fatal(testMsg)
	logger.Fatalf("%s", testMsg)

	require.Equal(t, 2, lco.lines)
}

func TestPanic(t *testing.T) {
	lco := &lineCounterOutput{}

	_, err := Init(LevelFromString("panic"), EncodingJSON, WithOutput(lco))
	require.Nil(t, err)

	logger := Logger().panicker(mockPanicker{t})
	logger.Debug(testMsg)
	logger.Debugf("%s", testMsg)
	logger.Info(testMsg)
	logger.Infof("%s", testMsg)
	logger.Warn(testMsg)
	logger.Warnf("%s", testMsg)
	logger.Error(testMsg)
	logger.Errorf("%s", testMsg)
	logger.Fatal(testMsg)
	logger.Fatalf("%s", testMsg)
	logger.Panic(testMsg)
	logger.Panicf("%s", testMsg)

	require.Equal(t, 2, lco.lines)
}

func TestLevelFromString(t *testing.T) {
	var tests = []struct {
		in       string
		expected slog.Level
	}{
		{"dEbug", LevelDebug},
		{"info", LevelInfo},
		{"WARN", LevelWarn},
		{"error", LevelError},
		{"fatal", LevelFatal},
		{"PANic", LevelPanic},
		{"", LevelUnknown},
		{"bubukitty", LevelUnknown},
	}

	for i, test := range tests {
		test := test
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			require.Equal(t, test.expected, LevelFromString(test.in))
		})
	}
}

func TestCustomLogMessages(t *testing.T) {
	_, err := Init(LevelFromString("info"), EncodingLogfmt, WithOutput(&testLogger{t}))
	if err != nil {
		t.Fatalf(err.Error())
	}

	t.Run("testformatting", func(t *testing.T) {
		logger := FromContext(context.Background())
		// the next two lines shouldn't show up due to level "info" in init
		logger.With("level_num", LevelDebug).Debug("f")
		logger.Debugf("f%d", LevelDebug)

		logger.With("level_num", LevelInfo).Info("f")
		logger.Infof("f%d", LevelInfo)

		logger.With("level_num", LevelWarn).Warn("f")
		logger.Warnf("f%d", LevelWarn)

		logger.With("level_num", LevelError).Error("f")
		logger.Errorf("f%d", LevelError)
	})

	t.Run("fatal", func(t *testing.T) {
		logger := FromContext(nil).exiter(mockExiter{t}) //nolint:staticcheck // SA1012
		logger.With("left", 42).Fatal("this my dearest friends, is where I leave you")
		logger.Fatalf("this my dearest friends, is where I leave %s", "you")
	})

	t.Run("panic", func(t *testing.T) {
		logger := FromContext(nil).panicker(mockPanicker{t}) //nolint:staticcheck // SA1012
		logger.With("left", 24).Panic("this my dearest friends, is where I leave you")
		logger.Panicf("this my dearest friends, is where I leave %s", "you")
	})
}

// tests the edge cases in context creation
func TestNewContext(t *testing.T) {
	ctx := WithFields(nil) //nolint:staticcheck // SA1012
	require.NotNil(t, ctx)

	ctx = WithFields(ctx, slog.Attr{})
	require.NotNil(t, ctx)

	ctx = WithFields(ctx, slog.String("key", "value"))
	require.NotNil(t, ctx)
}

func TestContextMethods(t *testing.T) {
	buf := &bytes.Buffer{}
	_, err := Init(LevelFromString("debug"), EncodingJSON, WithOutput(buf))
	require.Nil(t, err)

	logger := Logger()

	ctx := context.Background()
	logger.InfoContext(ctx, "test message", slog.String("key", "val"))
	require.Contains(t, buf.String(), "test message")
	require.Contains(t, buf.String(), "key")
	require.Contains(t, buf.String(), "val")

	buf.Reset()
	logger.DebugContext(ctx, "debug msg", slog.Int("n", 42))
	require.Contains(t, buf.String(), "debug msg")

	buf.Reset()
	logger.WarnContext(ctx, "warn msg")
	require.Contains(t, buf.String(), "warn msg")

	buf.Reset()
	logger.ErrorContext(ctx, "error msg", slog.String("err", "oops"))
	require.Contains(t, buf.String(), "error msg")
	require.Contains(t, buf.String(), "oops")
}

func TestSlogAccess(t *testing.T) {
	_, err := Init(LevelFromString("info"), EncodingJSON, WithOutput(io.Discard))
	require.Nil(t, err)

	logger := Logger()
	require.NotNil(t, logger.Slog())
}

func TestWithReplaceAttr(t *testing.T) {
	buf := &bytes.Buffer{}
	_, _, err := New(LevelInfo, EncodingJSON,
		WithOutput(buf),
		WithReplaceAttr(func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == "ts" {
				a.Key = "timestamp"
			}
			return a
		}),
	)
	require.Nil(t, err)
	// just verifying it doesn't panic
}

func BenchmarkSimpleLogging(b *testing.B) {
	_, err := Init(LevelFromString("info"), EncodingLogfmt,
		WithOutput(io.Discard),
	)
	if err != nil {
		b.Fatalf("failed to set up logger: %s", err)
	}

	logger := Logger()

	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		logger.Info("yeeeeeeeha!")
	}

	_ = logger
}

func BenchmarkSimpleLoggingWithCaller(b *testing.B) {
	_, err := Init(LevelFromString("info"), EncodingLogfmt,
		WithCaller(true),
		WithOutput(io.Discard),
	)
	if err != nil {
		b.Fatalf("failed to set up logger: %s", err)
	}

	logger := Logger()

	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		logger.Info("yeeeeeeeha!")
	}

	_ = logger
}

func BenchmarkJsonEncoding(b *testing.B) {
	_, err := Init(LevelFromString("info"), EncodingJSON, WithOutput(io.Discard))
	if err != nil {
		b.Fatalf("failed to set up logger: %s", err)
	}

	logger := Logger()

	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		logger.Info("yeeeeeeeha!")
	}

	_ = logger
}

func BenchmarkIgnoredLevel(b *testing.B) {
	_, err := Init(LevelFromString("info"), EncodingLogfmt, WithOutput(io.Discard))
	if err != nil {
		b.Fatalf("failed to set up logger: %s", err)
	}
	logger := Logger()

	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		logger.Debug("yeeeeeeeha!")
	}

	_ = logger
}

func BenchmarkWithAttributes(b *testing.B) {
	_, err := Init(LevelFromString("info"), EncodingLogfmt, WithOutput(io.Discard))
	if err != nil {
		b.Fatalf("failed to set up logger: %s", err)
	}
	logger := Logger()

	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		logger.With("leroy", "jenkins").Info("yeeeeeeeha!")
	}

	_ = logger
}

func BenchmarkSplitLevelHandler(b *testing.B) {
	_, err := Init(LevelFromString("info"), EncodingLogfmt, WithOutput(io.Discard), WithErrorOutput(io.Discard))
	if err != nil {
		b.Fatalf("failed to set up logger: %s", err)
	}
	logger := Logger()

	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		logger.Error("oh nooooo!")
	}

	_ = logger
}

func BenchmarkSplitLevelHandlerWithCaller(b *testing.B) {
	_, err := Init(LevelFromString("info"), EncodingLogfmt, WithOutput(io.Discard), WithErrorOutput(io.Discard),
		WithCaller(true),
	)
	if err != nil {
		b.Fatalf("failed to set up logger: %s", err)
	}
	logger := Logger()

	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		logger.Error("oh nooooo!")
	}

	_ = logger
}

func BenchmarkSplitLevelHandlerWithAttr(b *testing.B) {
	_, err := Init(LevelFromString("info"), EncodingLogfmt, WithOutput(io.Discard), WithErrorOutput(io.Discard))
	if err != nil {
		b.Fatalf("failed to set up logger: %s", err)
	}
	logger := Logger()

	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		logger.With("leroy", "jenkins").Error("oh nooooo!")
	}

	_ = logger
}

func BenchmarkContextLogging(b *testing.B) {
	_, err := Init(LevelFromString("info"), EncodingLogfmt, WithOutput(io.Discard))
	if err != nil {
		b.Fatalf("failed to set up logger: %s", err)
	}
	logger := Logger()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		logger.InfoContext(ctx, "message", slog.String("key", "val"))
	}

	_ = logger
}
