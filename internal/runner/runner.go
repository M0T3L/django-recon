package runner

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"time"
)

// CommandOptions defines configuration options for running a CLI command.
type CommandOptions struct {
	Timeout       time.Duration
	Dir           string
	Env           []string
	Stdin         io.Reader
	StdoutHandler LineHandler
	StderrHandler LineHandler
}

// Result holds execution output metrics and execution error.
type Result struct {
	ExitCode int
	Duration time.Duration
	Error    error
}

// Runner provides functionality to execute CLI commands safely.
type Runner struct{}

// New returns a new instance of Runner.
func New() *Runner {
	return &Runner{}
}

// Run executes a command safely with context timeout and panic isolation via recover().
func (r *Runner) Run(ctx context.Context, name string, args []string, opts CommandOptions) (res Result, err error) {
	// Panic isolation: prevent process panics from crashing the parent application
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("runner execution recovered from panic: %v", rec)
			res.Error = err
			res.ExitCode = -1
		}
	}()

	startTime := time.Now()

	// Apply timeout context if specified
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, name, args...)
	if opts.Dir != "" {
		cmd.Dir = opts.Dir
	}
	if len(opts.Env) > 0 {
		cmd.Env = opts.Env
	}
	if opts.Stdin != nil {
		cmd.Stdin = opts.Stdin
	}

	var stdoutPipe, stderrPipe io.ReadCloser
	if opts.StdoutHandler != nil {
		stdoutPipe, err = cmd.StdoutPipe()
		if err != nil {
			return Result{ExitCode: -1, Duration: time.Since(startTime), Error: err}, fmt.Errorf("failed to create stdout pipe: %w", err)
		}
	}

	if opts.StderrHandler != nil {
		stderrPipe, err = cmd.StderrPipe()
		if err != nil {
			return Result{ExitCode: -1, Duration: time.Since(startTime), Error: err}, fmt.Errorf("failed to create stderr pipe: %w", err)
		}
	}

	if err := cmd.Start(); err != nil {
		return Result{ExitCode: -1, Duration: time.Since(startTime), Error: err}, fmt.Errorf("failed to start command: %w", err)
	}

	errChan := make(chan error, 2)
	activeStreams := 0

	// Stream Stdout asynchronously if handler provided
	if stdoutPipe != nil {
		activeStreams++
		go func() {
			defer func() {
				if rec := recover(); rec != nil {
					errChan <- fmt.Errorf("stdout handler panicked: %v", rec)
				}
			}()
			errChan <- StreamOutput(stdoutPipe, opts.StdoutHandler)
		}()
	}

	// Stream Stderr asynchronously if handler provided
	if stderrPipe != nil {
		activeStreams++
		go func() {
			defer func() {
				if rec := recover(); rec != nil {
					errChan <- fmt.Errorf("stderr handler panicked: %v", rec)
				}
			}()
			errChan <- StreamOutput(stderrPipe, opts.StderrHandler)
		}()
	}

	// Collect stream errors concurrently while process runs / finishes
	var streamErr error
	for i := 0; i < activeStreams; i++ {
		if sErr := <-errChan; sErr != nil && streamErr == nil {
			streamErr = sErr
		}
	}

	// Wait for process completion
	cmdErr := cmd.Wait()
	res.Duration = time.Since(startTime)

	if streamErr != nil {
		res.Error = streamErr
		res.ExitCode = -1
		return res, streamErr
	}

	if cmdErr != nil {
		if ctx.Err() == context.DeadlineExceeded {
			res.Error = fmt.Errorf("command execution timed out after %v: %w", opts.Timeout, ctx.Err())
			res.ExitCode = -1
			return res, res.Error
		}

		if exitErr, ok := cmdErr.(*exec.ExitError); ok {
			res.ExitCode = exitErr.ExitCode()
		} else {
			res.ExitCode = -1
		}
		res.Error = cmdErr
		return res, cmdErr
	}

	res.ExitCode = 0
	return res, nil
}
