package command

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/grpc-ecosystem/go-grpc-middleware/logging/logrus/ctxlogrus"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/gitaly/v14/internal/testhelper"
)

func TestMain(m *testing.M) {
	testhelper.Run(m)
}

func TestNewCommandExtraEnv(t *testing.T) {
	ctx := testhelper.Context(t)

	extraVar := "FOOBAR=123456"
	buff := &bytes.Buffer{}
	cmd, err := New(ctx, exec.Command("/usr/bin/env"), nil, buff, nil, extraVar)

	require.NoError(t, err)
	require.NoError(t, cmd.Wait())

	require.Contains(t, strings.Split(buff.String(), "\n"), extraVar)
}

func TestNewCommandExportedEnv(t *testing.T) {
	ctx := testhelper.Context(t)

	testCases := []struct {
		key   string
		value string
	}{
		{
			key:   "HOME",
			value: "/home/git",
		},
		{
			key:   "PATH",
			value: "/usr/bin:/bin:/usr/sbin:/sbin:/usr/local/bin",
		},
		{
			key:   "LD_LIBRARY_PATH",
			value: "/path/to/your/lib",
		},
		{
			key:   "TZ",
			value: "foobar",
		},
		{
			key:   "GIT_TRACE",
			value: "true",
		},
		{
			key:   "GIT_TRACE_PACK_ACCESS",
			value: "true",
		},
		{
			key:   "GIT_TRACE_PACKET",
			value: "true",
		},
		{
			key:   "GIT_TRACE_PERFORMANCE",
			value: "true",
		},
		{
			key:   "GIT_TRACE_SETUP",
			value: "true",
		},
		{
			key:   "all_proxy",
			value: "http://localhost:4000",
		},
		{
			key:   "http_proxy",
			value: "http://localhost:5000",
		},
		{
			key:   "HTTP_PROXY",
			value: "http://localhost:6000",
		},
		{
			key:   "https_proxy",
			value: "https://localhost:5000",
		},
		{
			key:   "HTTPS_PROXY",
			value: "https://localhost:6000",
		},
		{
			key:   "no_proxy",
			value: "https://excluded:5000",
		},
		{
			key:   "NO_PROXY",
			value: "https://excluded:5000",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.key, func(t *testing.T) {
			if tc.key == "LD_LIBRARY_PATH" && runtime.GOOS == "darwin" {
				t.Skip("System Integrity Protection prevents using dynamic linker (dyld) environment variables on macOS. https://apple.co/2XDH4iC")
			}

			testhelper.ModifyEnvironment(t, tc.key, tc.value)

			buff := &bytes.Buffer{}
			cmd, err := New(ctx, exec.Command("/usr/bin/env"), nil, buff, nil)
			require.NoError(t, err)
			require.NoError(t, cmd.Wait())

			expectedEnv := fmt.Sprintf("%s=%s", tc.key, tc.value)
			require.Contains(t, strings.Split(buff.String(), "\n"), expectedEnv)
		})
	}
}

func TestNewCommandUnexportedEnv(t *testing.T) {
	ctx := testhelper.Context(t)

	unexportedEnvKey, unexportedEnvVal := "GITALY_UNEXPORTED_ENV", "foobar"
	testhelper.ModifyEnvironment(t, unexportedEnvKey, unexportedEnvVal)

	buff := &bytes.Buffer{}
	cmd, err := New(ctx, exec.Command("/usr/bin/env"), nil, buff, nil)

	require.NoError(t, err)
	require.NoError(t, cmd.Wait())

	require.NotContains(t, strings.Split(buff.String(), "\n"), fmt.Sprintf("%s=%s", unexportedEnvKey, unexportedEnvVal))
}

func TestRejectEmptyContextDone(t *testing.T) {
	defer func() {
		p := recover()
		if p == nil {
			t.Error("expected panic, got none")
			return
		}

		if _, ok := p.(contextWithoutDonePanic); !ok {
			panic(p)
		}
	}()

	_, err := New(testhelper.ContextWithoutCancel(), exec.Command("true"), nil, nil, nil)
	require.NoError(t, err)
}

func TestNewCommandTimeout(t *testing.T) {
	ctx := testhelper.Context(t)

	defer func(ch chan struct{}, t time.Duration) {
		spawnTokens = ch
		spawnConfig.Timeout = t
	}(spawnTokens, spawnConfig.Timeout)

	// This unbuffered channel will behave like a full/blocked buffered channel.
	spawnTokens = make(chan struct{})
	// Speed up the test by lowering the timeout
	spawnTimeout := 200 * time.Millisecond
	spawnConfig.Timeout = spawnTimeout

	testDeadline := time.After(1 * time.Second)
	tick := time.After(spawnTimeout / 2)

	errCh := make(chan error)
	go func() {
		_, err := New(ctx, exec.Command("true"), nil, nil, nil)
		errCh <- err
	}()

	var err error
	timePassed := false

wait:
	for {
		select {
		case err = <-errCh:
			break wait
		case <-tick:
			timePassed = true
		case <-testDeadline:
			t.Fatal("test timed out")
		}
	}

	require.True(t, timePassed, "time must have passed")
	require.Error(t, err)
	require.Contains(t, err.Error(), "process spawn timed out after")
}

func TestCommand_Wait_interrupts_after_context_cancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(testhelper.Context(t))

	cmd, err := New(ctx, exec.CommandContext(ctx, "sleep", "1h"), nil, nil, nil)
	require.NoError(t, err)

	// Cancel the command early.
	go cancel()

	err = cmd.Wait()
	require.Error(t, err)
	s, ok := ExitStatus(err)
	require.True(t, ok)
	require.Equal(t, -1, s)
}

func TestNewCommandWithSetupStdin(t *testing.T) {
	ctx := testhelper.Context(t)

	value := "Test value"
	output := bytes.NewBuffer(nil)

	cmd, err := New(ctx, exec.Command("cat"), SetupStdin, nil, nil)
	require.NoError(t, err)

	_, err = fmt.Fprintf(cmd, "%s", value)
	require.NoError(t, err)

	// The output of the `cat` subprocess should exactly match its input
	_, err = io.CopyN(output, cmd, int64(len(value)))
	require.NoError(t, err)
	require.Equal(t, value, output.String())

	require.NoError(t, cmd.Wait())
}

func TestNewCommandNullInArg(t *testing.T) {
	ctx := testhelper.Context(t)

	_, err := New(ctx, exec.Command("sh", "-c", "hello\x00world"), nil, nil, nil)
	require.Error(t, err)
	require.EqualError(t, err, `detected null byte in command argument "hello\x00world"`)
}

func TestNewNonExistent(t *testing.T) {
	ctx := testhelper.Context(t)

	cmd, err := New(ctx, exec.Command("command-non-existent"), nil, nil, nil)
	require.Nil(t, cmd)
	require.Error(t, err)
}

func TestCommandStdErr(t *testing.T) {
	ctx := testhelper.Context(t)

	var stdout, stderr bytes.Buffer
	expectedMessage := `hello world\nhello world\nhello world\nhello world\nhello world\n`

	logger := logrus.New()
	logger.SetOutput(&stderr)

	ctx = ctxlogrus.ToContext(ctx, logrus.NewEntry(logger))

	cmd, err := New(ctx, exec.Command("./testdata/stderr_script.sh"), nil, &stdout, nil)
	require.NoError(t, err)
	require.Error(t, cmd.Wait())

	assert.Empty(t, stdout.Bytes())
	require.Equal(t, expectedMessage, extractLastMessage(stderr.String()))
}

func TestCommandStdErrLargeOutput(t *testing.T) {
	ctx := testhelper.Context(t)

	var stdout, stderr bytes.Buffer

	logger := logrus.New()
	logger.SetOutput(&stderr)

	ctx = ctxlogrus.ToContext(ctx, logrus.NewEntry(logger))

	cmd, err := New(ctx, exec.Command("./testdata/stderr_many_lines.sh"), nil, &stdout, nil)
	require.NoError(t, err)
	require.Error(t, cmd.Wait())

	assert.Empty(t, stdout.Bytes())
	msg := strings.ReplaceAll(extractLastMessage(stderr.String()), "\\n", "\n")
	require.LessOrEqual(t, len(msg), maxStderrBytes)
}

func TestCommandStdErrBinaryNullBytes(t *testing.T) {
	ctx := testhelper.Context(t)

	var stdout, stderr bytes.Buffer

	logger := logrus.New()
	logger.SetOutput(&stderr)

	ctx = ctxlogrus.ToContext(ctx, logrus.NewEntry(logger))

	cmd, err := New(ctx, exec.Command("./testdata/stderr_binary_null.sh"), nil, &stdout, nil)
	require.NoError(t, err)
	require.Error(t, cmd.Wait())

	assert.Empty(t, stdout.Bytes())
	msg := strings.SplitN(extractLastMessage(stderr.String()), "\\n", 2)[0]
	require.Equal(t, strings.Repeat("\\x00", maxStderrLineLength), msg)
}

func TestCommandStdErrLongLine(t *testing.T) {
	ctx := testhelper.Context(t)

	var stdout, stderr bytes.Buffer

	logger := logrus.New()
	logger.SetOutput(&stderr)

	ctx = ctxlogrus.ToContext(ctx, logrus.NewEntry(logger))

	cmd, err := New(ctx, exec.Command("./testdata/stderr_repeat_a.sh"), nil, &stdout, nil)
	require.NoError(t, err)
	require.Error(t, cmd.Wait())

	assert.Empty(t, stdout.Bytes())
	require.Contains(t, stderr.String(), fmt.Sprintf("%s\\n%s", strings.Repeat("a", maxStderrLineLength), strings.Repeat("b", maxStderrLineLength)))
}

func TestCommandStdErrMaxBytes(t *testing.T) {
	ctx := testhelper.Context(t)

	var stdout, stderr bytes.Buffer

	logger := logrus.New()
	logger.SetOutput(&stderr)

	ctx = ctxlogrus.ToContext(ctx, logrus.NewEntry(logger))

	cmd, err := New(ctx, exec.Command("./testdata/stderr_max_bytes_edge_case.sh"), nil, &stdout, nil)
	require.NoError(t, err)
	require.Error(t, cmd.Wait())

	assert.Empty(t, stdout.Bytes())
	message := extractLastMessage(stderr.String())
	require.Equal(t, maxStderrBytes, len(strings.ReplaceAll(message, "\\n", "\n")))
}

var logMsgRegex = regexp.MustCompile(`msg="(.+?)"`)

func extractLastMessage(logMessage string) string {
	subMatchesAll := logMsgRegex.FindAllStringSubmatch(logMessage, -1)
	if len(subMatchesAll) < 1 {
		return ""
	}

	subMatches := subMatchesAll[len(subMatchesAll)-1]
	if len(subMatches) != 2 {
		return ""
	}

	return subMatches[1]
}

func TestCommand_logMessage(t *testing.T) {
	logger, hook := test.NewNullLogger()
	logger.SetLevel(logrus.DebugLevel)

	ctx := ctxlogrus.ToContext(testhelper.Context(t), logrus.NewEntry(logger))

	cmd, err := New(ctx, exec.Command("echo", "hello world"), nil, nil, nil)
	require.NoError(t, err)
	cgroupPath := "/sys/fs/cgroup/1"
	cmd.SetCgroupPath(cgroupPath)

	require.NoError(t, cmd.Wait())
	logEntry := hook.LastEntry()
	assert.Equal(t, cmd.Pid(), logEntry.Data["pid"])
	assert.Equal(t, []string{"echo", "hello world"}, logEntry.Data["args"])
	assert.Equal(t, 0, logEntry.Data["command.exitCode"])
	assert.Equal(t, cgroupPath, logEntry.Data["command.cgroup_path"])
}
