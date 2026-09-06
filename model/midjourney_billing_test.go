package model

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/require"
)

func TestPersistMidjourneySubmissionLinkResolvesCommittedReplyExactly(t *testing.T) {
	for _, fixture := range groupReservationDatabases(t) {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			db := useGroupReservationDatabase(t, fixture)
			require.NoError(t, db.AutoMigrate(&Task{}, &TaskSubmission{}, &Midjourney{}))

			identity := fmt.Sprintf("mj-link-%s-%d", fixture.name, time.Now().UnixNano())
			submission := &TaskSubmission{
				SubmissionID: identity, State: TaskSubmissionStateActive,
				LeaseToken: identity + "-lease", UserID: 41,
			}
			require.NoError(t, db.Create(submission).Error)
			task := &Task{
				TaskID: identity + "-public", Platform: constant.TaskPlatformMidjourney,
				UserId: 41, ChannelId: 7, Action: constant.MjActionImagine,
				Status: TaskStatusSubmitted, Progress: "0%",
				PrivateData: TaskPrivateData{UpstreamTaskID: identity + "-upstream"},
			}
			midjourney := &Midjourney{
				UserId: 41, ChannelId: 7, Action: constant.MjActionImagine,
				MjId: task.PrivateData.UpstreamTaskID, Quota: 300, Progress: "0%",
			}

			pool := installTaskSubmissionCommitErrorPool(t, db)
			pool.failCommit = true
			require.NoError(t, PersistMidjourneySubmissionLink(context.Background(), submission.SubmissionID, submission.LeaseToken, midjourney, task))
			require.Positive(t, task.ID)
			require.Positive(t, midjourney.Id)
			require.NotNil(t, midjourney.TaskRowID)
			require.Equal(t, task.ID, *midjourney.TaskRowID)

			var storedSubmission TaskSubmission
			require.NoError(t, db.Where("submission_id = ?", submission.SubmissionID).First(&storedSubmission).Error)
			require.Equal(t, TaskSubmissionStateActive, storedSubmission.State)
			require.NotNil(t, storedSubmission.TaskRowID)
			require.Equal(t, task.ID, *storedSubmission.TaskRowID)
			require.Equal(t, task.PrivateData.UpstreamTaskID, midjourney.MjId)

			mismatchedTask := *task
			mismatchedTask.PrivateData.UpstreamTaskID = common.GetUUID()
			require.Error(t, verifyMidjourneySubmissionLink(context.Background(), submission.SubmissionID, submission.LeaseToken, midjourney, &mismatchedTask))

			newTask := *task
			newTask.ID = 0
			newTask.TaskID = identity + "-mismatch"
			newMidjourney := *midjourney
			newMidjourney.Id = 0
			newMidjourney.TaskRowID = nil
			newMidjourney.MjId = identity + "-different-upstream"
			require.Error(t, PersistMidjourneySubmissionLink(context.Background(), submission.SubmissionID, submission.LeaseToken, &newMidjourney, &newTask))
		})
	}
}
