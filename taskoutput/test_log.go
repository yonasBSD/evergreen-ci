package taskoutput

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/log"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
)

// TestLogOutput is the versioned entry point for coordinating persistent
// storage of a task run's test log data.
type TestLogOutput struct {
	Version      int                    `bson:"version" json:"version"`
	BucketConfig evergreen.BucketConfig `bson:"bucket_config" json:"bucket_config"`

	AWSCredentials aws.CredentialsProvider `bson:"-" json:"-"`
}

// ID returns the unique identifier of the test log output type.
func (TestLogOutput) ID() string { return "test_logs" }

// TestLogGetOptions represents the arguments for fetching test logs belonging
// to a task run.
type TestLogGetOptions struct {
	// LogPaths are the paths of the logs to fetch and merge, prefixes may
	// be specified. At least one value must be specified.
	LogPaths []string
	// Start is the start time (inclusive) of the time range filter,
	// represented as a Unix timestamp in nanoseconds. Defaults to the
	// first timestamp of the first specified log.
	Start *int64
	// End is the end time (inclusive) of the time range filter,
	// represented as a Unix timestamp in nanoseconds. Defaults to the last
	// timestamp of the first specified log.
	End *int64
	// LineLimit limits the number of lines read from the log. Ignored if
	// less than or equal to 0.
	LineLimit int
	// TailN is the number of lines to read from the tail of the log.
	// Ignored if less than or equal to 0.
	TailN int
}

// NewSender returns a new test log sender for the given task run.
func (o TestLogOutput) NewSender(ctx context.Context, taskOpts TaskOptions, senderOpts EvergreenSenderOptions, logPath string, sequence int) (send.Sender, error) {
	svc, err := o.getLogService(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "getting log service")
	}

	senderOpts.appendLines = func(ctx context.Context, lines []log.LogLine) error {
		return svc.Append(ctx, o.getLogNames(taskOpts, []string{logPath})[0], sequence, lines)
	}

	return newEvergreenSender(ctx, fmt.Sprintf("%s-%s", taskOpts.TaskID, logPath), senderOpts)
}

// Append appends log lines to the specified test log for the given task run.
func (o TestLogOutput) Append(ctx context.Context, taskOpts TaskOptions, logPath string, lines []log.LogLine) error {
	svc, err := o.getLogService(ctx)
	if err != nil {
		return errors.Wrap(err, "getting log service")
	}

	return svc.Append(ctx, o.getLogNames(taskOpts, []string{logPath})[0], 0, lines)
}

// Get returns test logs belonging to the specified task run.
func (o TestLogOutput) Get(ctx context.Context, taskOpts TaskOptions, getOpts TestLogGetOptions) (log.LogIterator, error) {
	if o.Version == 0 {
		return o.getBuildloggerLogs(ctx, taskOpts, getOpts)
	}

	svc, err := o.getLogService(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "getting log service")
	}

	return svc.Get(ctx, log.GetOptions{
		LogNames:                   o.getLogNames(taskOpts, getOpts.LogPaths),
		Start:                      getOpts.Start,
		End:                        getOpts.End,
		DefaultTimeRangeOfFirstLog: true,
		LineLimit:                  getOpts.LineLimit,
		TailN:                      getOpts.TailN,
	})
}

func (o TestLogOutput) getLogNames(taskOpts TaskOptions, logPaths []string) []string {
	prefix := fmt.Sprintf("%s/%s/%d/%s", taskOpts.ProjectID, taskOpts.TaskID, taskOpts.Execution, o.ID())

	logNames := make([]string, len(logPaths))
	for i, path := range logPaths {
		logNames[i] = fmt.Sprintf("%s/%s", prefix, path)
	}

	return logNames
}

func (o TestLogOutput) getLogService(ctx context.Context) (log.LogService, error) {
	b, err := newBucket(ctx, o.BucketConfig, o.AWSCredentials)
	if err != nil {
		return nil, err
	}

	return log.NewLogServiceV0(b), nil
}

// getBuildloggerLogs makes request to Cedar Buildlogger for logs.
func (o TestLogOutput) getBuildloggerLogs(ctx context.Context, taskOpts TaskOptions, getOpts TestLogGetOptions) (log.LogIterator, error) {
	if len(getOpts.LogPaths) != 1 {
		return nil, errors.New("must request exactly one test log from Cedar Buildlogger")
	}

	return apimodels.GetBuildloggerLogs(ctx, apimodels.GetBuildloggerLogsOptions{
		BaseURL:   evergreen.GetEnvironment().Settings().Cedar.BaseURL,
		TaskID:    taskOpts.TaskID,
		Execution: utility.ToIntPtr(taskOpts.Execution),
		TestName:  getOpts.LogPaths[0],
		Start:     utility.FromInt64Ptr(getOpts.Start),
		End:       utility.FromInt64Ptr(getOpts.End),
		Limit:     getOpts.LineLimit,
		Tail:      getOpts.TailN,
	})
}
