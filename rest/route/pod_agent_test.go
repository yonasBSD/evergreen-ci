package route

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/event"
	patchmodel "github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/evergreen/model/pod/dispatcher"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPodProvisioningScript(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	getRoute := func(t *testing.T, env evergreen.Environment, podID string) *podProvisioningScript {
		rh := makePodProvisioningScript(env)
		r, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
		require.NoError(t, err)
		r = gimlet.SetURLVars(r, map[string]string{"pod_id": podID})
		require.NoError(t, rh.Parse(ctx, r))
		pps, ok := rh.(*podProvisioningScript)
		require.True(t, ok, "route should be pod provisioning script route")
		return pps
	}

	t.Run("RunFailsWithNonexistentPod", func(t *testing.T) {
		env := mock.Environment{
			EvergreenSettings: &evergreen.Settings{
				Api: evergreen.APIConfig{URL: "www.test.com"},
			},
			Clients: evergreen.ClientConfig{S3URLPrefix: "https://foo.com"},
		}
		rh := getRoute(t, &env, "nonexistent")
		resp := rh.Run(ctx)
		assert.NotEqual(t, http.StatusOK, resp.Status())
	})

	t.Run("RunGeneratesScriptSuccessfully", func(t *testing.T) {
		for tName, tCase := range map[string]func(t *testing.T, env evergreen.Environment, p *pod.Pod){
			"ClientDownloadsWithLinuxPod": func(t *testing.T, env evergreen.Environment, p *pod.Pod) {
				require.NoError(t, p.Insert(t.Context()))

				rh := getRoute(t, env, p.ID)
				resp := rh.Run(ctx)
				assert.Equal(t, http.StatusOK, resp.Status())

				script, ok := resp.Data().(string)
				require.True(t, ok, "route should return plaintext response")

				expected := "curl -fLO https://foo.com/linux_amd64/evergreen --retry 10 --retry-max-time 100 && " +
					"chmod +x evergreen && " +
					"./evergreen agent --api_server=www.test.com --mode=pod --log_output=file --log_prefix=/working/dir/agent --working_directory=/working/dir"
				assert.Equal(t, expected, script)
			},
			"ClientDownloadsWithWindowsPod": func(t *testing.T, env evergreen.Environment, p *pod.Pod) {
				p.TaskContainerCreationOpts.OS = pod.OSWindows
				require.NoError(t, p.Insert(t.Context()))

				rh := getRoute(t, env, p.ID)
				resp := rh.Run(ctx)
				assert.Equal(t, http.StatusOK, resp.Status())

				script, ok := resp.Data().(string)
				require.True(t, ok, "route should return plaintext response")

				expected := "curl.exe -fLO https://foo.com/windows_amd64/evergreen.exe --retry 10 --retry-max-time 100; " +
					".\\evergreen.exe agent --api_server=www.test.com --mode=pod --log_output=file --log_prefix=/working/dir/agent --working_directory=/working/dir"
				assert.Equal(t, expected, script)
			},
		} {
			t.Run(tName, func(t *testing.T) {
				require.NoError(t, db.ClearCollections(pod.Collection))
				p := &pod.Pod{
					ID: "id",
					TaskContainerCreationOpts: pod.TaskContainerCreationOptions{
						OS:         pod.OSLinux,
						Arch:       pod.ArchAMD64,
						WorkingDir: "/working/dir",
					},
					Status: pod.StatusStarting,
					TimeInfo: pod.TimeInfo{
						Initializing: time.Now().Add(-2 * time.Minute),
						Starting:     time.Now().Add(-time.Minute),
					},
				}
				env := mock.Environment{
					EvergreenSettings: &evergreen.Settings{
						Api: evergreen.APIConfig{URL: "www.test.com"},
					},
					Clients: evergreen.ClientConfig{S3URLPrefix: "https://foo.com"},
				}
				tCase(t, &env, p)
			})
		}
	})
}

func TestPodAgentNextTask(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(task.Collection, pod.Collection, dispatcher.Collection, model.ProjectRefCollection, event.EventCollection))
	}()
	getPod := func() pod.Pod {
		return pod.Pod{
			ID:     "pod1",
			Status: pod.StatusStarting,
		}
	}
	getTask := func() task.Task {
		return task.Task{
			Id:                 "t1",
			Project:            "proj",
			ContainerAllocated: true,
			ExecutionPlatform:  task.ExecutionPlatformContainer,
			Status:             evergreen.TaskUndispatched,
			Activated:          true,
		}
	}
	getProject := func() model.ProjectRef {
		return model.ProjectRef{
			Id:      "proj",
			Enabled: true,
		}
	}
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, rh *podAgentNextTask, env evergreen.Environment){
		"ParseSetsPodID": func(ctx context.Context, t *testing.T, rh *podAgentNextTask, env evergreen.Environment) {
			r, err := http.NewRequest(http.MethodGet, "/url", nil)
			require.NoError(t, err)
			podID := "some_pod_id"
			r = gimlet.SetURLVars(r, map[string]string{"pod_id": podID})
			require.NoError(t, rh.Parse(ctx, r))
			assert.Equal(t, podID, rh.podID)
		},
		"ParseFailsWithoutPodID": func(ctx context.Context, t *testing.T, rh *podAgentNextTask, env evergreen.Environment) {
			r, err := http.NewRequest(http.MethodGet, "/url", nil)
			require.NoError(t, err)
			assert.Error(t, rh.Parse(ctx, r))
			assert.Zero(t, rh.podID)
		},
		"RunFailsWithNonexistentPod": func(ctx context.Context, t *testing.T, rh *podAgentNextTask, env evergreen.Environment) {
			tsk := getTask()
			require.NoError(t, tsk.Insert(t.Context()))
			d := dispatcher.NewPodDispatcher("group", []string{tsk.Id}, []string{"nonexistent_pod"})
			require.NoError(t, d.Insert(t.Context()))
			rh.podID = "nonexistent_pod"
			resp := rh.Run(ctx)
			assert.Equal(t, http.StatusNotFound, resp.Status())
		},
		"RunFailsWithNonexistentDispatcher": func(ctx context.Context, t *testing.T, rh *podAgentNextTask, env evergreen.Environment) {
			p := getPod()
			require.NoError(t, p.Insert(t.Context()))
			rh.podID = p.ID

			resp := rh.Run(ctx)
			assert.Equal(t, http.StatusNotFound, resp.Status())
		},
		"RunShouldEnqueueTerminationJobWithNonRunningPod": func(ctx context.Context, t *testing.T, rh *podAgentNextTask, env evergreen.Environment) {
			p := getPod()
			p.Status = pod.StatusTerminated
			require.NoError(t, p.Insert(t.Context()))
			rh.podID = p.ID

			resp := rh.Run(ctx)
			assert.Equal(t, http.StatusOK, resp.Status())
			stats := env.RemoteQueue().Stats(ctx)
			assert.Equal(t, 1, stats.Total)
		},
		"RunCorrectlyMarksContainerTaskDispatched": func(ctx context.Context, t *testing.T, rh *podAgentNextTask, env evergreen.Environment) {
			p := getPod()
			require.NoError(t, p.Insert(t.Context()))
			proj := getProject()
			require.NoError(t, proj.Insert(t.Context()))
			tsk := getTask()
			require.NoError(t, tsk.Insert(t.Context()))
			d := dispatcher.NewPodDispatcher("group", []string{tsk.Id}, []string{p.ID})
			require.NoError(t, d.Insert(t.Context()))
			rh.podID = p.ID

			resp := rh.Run(ctx)
			assert.Equal(t, http.StatusOK, resp.Status())
			nextTaskResp, ok := resp.Data().(*apimodels.NextTaskResponse)
			require.True(t, ok)
			assert.Equal(t, nextTaskResp.TaskId, tsk.Id)
			foundTask, err := task.FindOneId(ctx, tsk.Id)
			require.NoError(t, err)
			assert.Equal(t, evergreen.TaskDispatched, foundTask.Status)
		},
		"RunPreparesToTerminatePodWhenThereAreNoTasksToDispatch": func(ctx context.Context, t *testing.T, rh *podAgentNextTask, env evergreen.Environment) {
			p := getPod()
			p.TimeInfo.Initializing = time.Now()
			require.NoError(t, p.Insert(t.Context()))
			rh.podID = p.ID

			d := dispatcher.NewPodDispatcher("group", []string{}, []string{p.ID})
			require.NoError(t, d.Insert(t.Context()))

			resp := rh.Run(ctx)
			assert.Equal(t, http.StatusOK, resp.Status())

			dbPod, err := pod.FindOneByID(ctx, p.ID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.NotZero(t, dbPod.TimeInfo.AgentStarted)
			assert.Equal(t, pod.StatusDecommissioned, dbPod.Status)

			stats := env.RemoteQueue().Stats(ctx)
			assert.Equal(t, 1, stats.Total)
		},
		"RunUpdatesStartingPodToDecommissionedAfterTaskDispatch": func(ctx context.Context, t *testing.T, rh *podAgentNextTask, env evergreen.Environment) {
			p := getPod()
			p.TimeInfo.Initializing = time.Now()
			require.NoError(t, p.Insert(t.Context()))
			assert.Equal(t, pod.StatusStarting, p.Status, "initial pod status should be starting")
			rh.podID = p.ID

			proj := getProject()
			require.NoError(t, proj.Insert(t.Context()))
			tsk := getTask()
			require.NoError(t, tsk.Insert(t.Context()))
			d := dispatcher.NewPodDispatcher("group", []string{tsk.Id}, []string{p.ID})
			require.NoError(t, d.Insert(t.Context()))

			resp := rh.Run(ctx)
			assert.Equal(t, http.StatusOK, resp.Status())

			dbPod, err := pod.FindOneByID(ctx, p.ID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.NotZero(t, dbPod.TimeInfo.AgentStarted)
			assert.Equal(t, pod.StatusDecommissioned, dbPod.Status)
			assert.Equal(t, tsk.Id, dbPod.TaskRuntimeInfo.RunningTaskID)
			assert.Equal(t, tsk.Execution, dbPod.TaskRuntimeInfo.RunningTaskExecution)
		},
		"RunReturnsRunningTaskIfItExists": func(ctx context.Context, t *testing.T, rh *podAgentNextTask, env evergreen.Environment) {
			tsk := getTask()
			require.NoError(t, tsk.Insert(t.Context()))
			p := getPod()
			p.TaskRuntimeInfo.RunningTaskID = tsk.Id
			p.TaskRuntimeInfo.RunningTaskExecution = tsk.Execution
			require.NoError(t, p.Insert(t.Context()))

			rh.podID = p.ID
			resp := rh.Run(ctx)
			assert.Equal(t, http.StatusOK, resp.Status())
			nextTaskResp, ok := resp.Data().(*apimodels.NextTaskResponse)
			require.True(t, ok)
			assert.Equal(t, nextTaskResp.TaskId, tsk.Id)
		},
		"DegradedModeSetShouldTerminatePod": func(ctx context.Context, t *testing.T, rh *podAgentNextTask, env evergreen.Environment) {
			defer func() {
				// unset degraded mode
				require.NoError(t, evergreen.SetServiceFlags(ctx, evergreen.ServiceFlags{
					TaskDispatchDisabled: false,
				}))
			}()
			require.NoError(t, evergreen.SetServiceFlags(ctx, evergreen.ServiceFlags{
				TaskDispatchDisabled: true,
			}))
			p := getPod()
			require.NoError(t, p.Insert(t.Context()))
			tsk := getTask()
			require.NoError(t, tsk.Insert(t.Context()))
			rh.podID = p.ID
			resp := rh.Run(ctx)
			nextTaskResp, ok := resp.Data().(*apimodels.NextTaskResponse)
			require.True(t, ok)
			assert.Equal(t, &apimodels.NextTaskResponse{}, nextTaskResp)

			q := env.RemoteQueue()
			require.NoError(t, q.Start(ctx))
			require.True(t, amboy.WaitInterval(ctx, q, time.Millisecond))
			stats := q.Stats(ctx)
			assert.Equal(t, 1, stats.Total)

			dbPod, err := pod.FindOneByID(ctx, rh.podID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.Equal(t, pod.StatusTerminated, dbPod.Status)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			env := &mock.Environment{}
			require.NoError(t, env.Configure(ctx))
			// Don't use the default local limited size remote queue from the
			// mock env because it does not accept jobs when it's not active.
			rq, err := queue.NewLocalLimitedSizeSerializable(1, 1)
			require.NoError(t, err)
			env.Remote = rq

			require.NoError(t, db.ClearCollections(task.Collection, pod.Collection, dispatcher.Collection, model.ProjectRefCollection, event.EventCollection))

			rh, ok := makePodAgentNextTask(env).(*podAgentNextTask)
			require.True(t, ok)

			tCase(ctx, t, rh, env)
		})
	}
}

func TestPodAgentEndTask(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(task.Collection, pod.Collection, event.EventCollection, model.ProjectRefCollection, build.Collection, model.VersionCollection, patchmodel.Collection))
	}()
	const (
		podID         = "pod"
		taskID        = "task"
		taskExecution = 2
		buildID       = "build"
		versionID     = "version"
		projID        = "proj"
	)
	td := &apimodels.TaskEndDetail{
		Status: evergreen.TaskSucceeded,
	}
	jsonBody, err := json.Marshal(td)
	require.NoError(t, err)
	buffer := bytes.NewBuffer(jsonBody)
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, rh *podAgentEndTask, env evergreen.Environment){
		"ParseSetsPodAndTaskID": func(ctx context.Context, t *testing.T, rh *podAgentEndTask, env evergreen.Environment) {
			r, err := http.NewRequest(http.MethodPost, "/url", buffer)
			require.NoError(t, err)
			podID := "some_pod_id"
			taskID := "some_task_id"
			r = gimlet.SetURLVars(r, map[string]string{"pod_id": podID, "task_id": taskID})
			require.NoError(t, rh.Parse(ctx, r))
			assert.Equal(t, podID, rh.podID)
			assert.Equal(t, taskID, rh.taskID)
		},
		"ParseFailsWithoutPodIDOrTaskID": func(ctx context.Context, t *testing.T, rh *podAgentEndTask, env evergreen.Environment) {
			r, err := http.NewRequest(http.MethodPost, "/url", nil)
			require.NoError(t, err)
			podID := "some_pod_id"
			taskID := "some_task_id"
			r = gimlet.SetURLVars(r, map[string]string{"task_id": taskID})
			assert.Error(t, rh.Parse(ctx, r))
			assert.Zero(t, rh.podID)
			r = gimlet.SetURLVars(r, map[string]string{"pod_id": podID})
			assert.Error(t, rh.Parse(ctx, r))
			assert.Zero(t, rh.taskID)
		},
		"RunFailsWithNonexistentPodOrTask": func(ctx context.Context, t *testing.T, rh *podAgentEndTask, env evergreen.Environment) {
			rh.podID = podID
			rh.taskID = taskID
			resp := rh.Run(ctx)
			assert.Equal(t, http.StatusNotFound, resp.Status())
			podToInsert := &pod.Pod{
				ID: podID,
			}
			require.NoError(t, podToInsert.Insert(t.Context()))
			resp = rh.Run(ctx)
			assert.Equal(t, http.StatusNotFound, resp.Status())
		},
		"RunNoOpsWithNilRunningTaskAndInvalidStatus": func(ctx context.Context, t *testing.T, rh *podAgentEndTask, env evergreen.Environment) {
			podToInsert := &pod.Pod{
				ID: podID,
				TaskRuntimeInfo: pod.TaskRuntimeInfo{
					RunningTaskID:        "",
					RunningTaskExecution: 0,
				},
			}
			require.NoError(t, podToInsert.Insert(t.Context()))
			taskToInsert := &task.Task{
				Id:        taskID,
				Execution: taskExecution,
			}
			require.NoError(t, taskToInsert.Insert(t.Context()))
			rh.podID = podID
			rh.taskID = taskID
			resp := rh.Run(ctx)
			endTaskResp, ok := resp.Data().(*apimodels.EndTaskResponse)
			require.True(t, ok)
			assert.Equal(t, &apimodels.EndTaskResponse{}, endTaskResp)
			require.NoError(t, podToInsert.UpdateStatus(ctx, pod.StatusStarting, ""))
			resp = rh.Run(ctx)
			endTaskResp, ok = resp.Data().(*apimodels.EndTaskResponse)
			require.True(t, ok)
			assert.Equal(t, &apimodels.EndTaskResponse{}, endTaskResp)
		},
		"RunSuccessfullyFinishesTask": func(ctx context.Context, t *testing.T, rh *podAgentEndTask, env evergreen.Environment) {
			podToInsert := &pod.Pod{
				ID:     podID,
				Status: pod.StatusRunning,
				TaskRuntimeInfo: pod.TaskRuntimeInfo{
					RunningTaskID:        taskID,
					RunningTaskExecution: taskExecution,
				},
			}
			require.NoError(t, podToInsert.Insert(t.Context()))
			taskToInsert := &task.Task{
				Id:                taskID,
				Execution:         taskExecution,
				BuildId:           buildID,
				Version:           versionID,
				Project:           projID,
				PodID:             podID,
				ExecutionPlatform: task.ExecutionPlatformContainer,
			}
			require.NoError(t, taskToInsert.Insert(t.Context()))
			buildToInsert := &build.Build{
				Id:      buildID,
				Project: projID,
			}
			require.NoError(t, buildToInsert.Insert(t.Context()))
			versionToInsert := &model.Version{
				Id: versionID,
			}
			require.NoError(t, versionToInsert.Insert(t.Context()))
			projectToInsert := model.ProjectRef{
				Id:         projID,
				Identifier: "identifier",
			}
			require.NoError(t, projectToInsert.Insert(t.Context()))
			parserProjectToInsert := model.ParserProject{
				Id: versionToInsert.Id,
			}
			require.NoError(t, parserProjectToInsert.Insert(t.Context()))
			rh.podID = podID
			rh.taskID = taskID
			rh.details = *td
			resp := rh.Run(ctx)
			endTaskResp, ok := resp.Data().(*apimodels.EndTaskResponse)
			require.True(t, ok)
			assert.False(t, endTaskResp.ShouldExit)
			foundTask, err := task.FindOneId(ctx, taskID)
			require.NoError(t, err)
			assert.Equal(t, evergreen.TaskSucceeded, foundTask.Status)
		},
		"RunNoOpsOnAbortedTask": func(ctx context.Context, t *testing.T, rh *podAgentEndTask, env evergreen.Environment) {
			podToInsert := &pod.Pod{
				ID:     podID,
				Status: pod.StatusRunning,
				TaskRuntimeInfo: pod.TaskRuntimeInfo{
					RunningTaskID:        taskID,
					RunningTaskExecution: taskExecution,
				},
			}
			require.NoError(t, podToInsert.Insert(t.Context()))
			taskToInsert := &task.Task{
				Id:                taskID,
				Execution:         taskExecution,
				BuildId:           buildID,
				Version:           versionID,
				Project:           projID,
				PodID:             podID,
				ExecutionPlatform: task.ExecutionPlatformContainer,
			}
			require.NoError(t, taskToInsert.Insert(t.Context()))
			buildToInsert := &build.Build{
				Id:      buildID,
				Project: projID,
			}
			require.NoError(t, buildToInsert.Insert(t.Context()))
			versionToInsert := &model.Version{
				Id: versionID,
			}
			require.NoError(t, versionToInsert.Insert(t.Context()))
			projectToInsert := model.ProjectRef{
				Id:         projID,
				Identifier: "identifier",
			}
			require.NoError(t, projectToInsert.Insert(t.Context()))
			parserProjectToInsert := model.ParserProject{
				Id: versionToInsert.Id,
			}
			require.NoError(t, parserProjectToInsert.Insert(t.Context()))
			rh.podID = podID
			rh.taskID = taskID
			rh.details = *td
			rh.details.Status = evergreen.TaskUndispatched
			resp := rh.Run(ctx)
			endTaskResp, ok := resp.Data().(*apimodels.EndTaskResponse)
			require.True(t, ok)
			assert.Equal(t, &apimodels.EndTaskResponse{}, endTaskResp)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			env := &mock.Environment{}
			require.NoError(t, env.Configure(ctx))

			require.NoError(t, db.ClearCollections(task.Collection, pod.Collection, event.EventCollection, model.ProjectRefCollection, build.Collection, model.VersionCollection, patchmodel.Collection, model.ParserProjectCollection))

			rh, ok := makePodAgentEndTask(env).(*podAgentEndTask)
			require.True(t, ok)

			tCase(ctx, t, rh, env)
		})
	}
}
