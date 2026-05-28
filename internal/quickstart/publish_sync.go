package quickstart

import (
	"fmt"

	"github.com/wujun4code/clashforge/internal/publish"
)

type quickStartPublishSyncInput struct {
	WorkerName   string
	WorkerURL    string
	WorkerDevURL string
	Hostname     string
	AccountID    string
	NamespaceID  string
	ZoneID       string
	AccessToken  string
	BaseName     string
	Version      int
	FileName     string
	AccessURL    string
}

// syncQuickStartPublishArtifacts persists quickstart-created publish resources
// so they are visible in /publish "私有仓库" and "已发布的订阅".
func syncQuickStartPublishArtifacts(store *publish.Store, out EventWriter, in quickStartPublishSyncInput) {
	if store == nil {
		emit(out, PhasePublish, "sync_publish_store", StatusWarning,
			"订阅已发布，但发布中心未初始化，无法同步到 /publish")
		return
	}

	cfg, err := store.UpsertWorkerConfig(publish.WorkerConfigInput{
		Name:         in.WorkerName,
		WorkerName:   in.WorkerName,
		WorkerURL:    in.WorkerURL,
		WorkerDevURL: in.WorkerDevURL,
		Hostname:     in.Hostname,
		AccountID:    in.AccountID,
		NamespaceID:  in.NamespaceID,
		ZoneID:       in.ZoneID,
		// Important: this must be the publish worker access token
		// so /publish upload/delete keeps working.
		Token: in.AccessToken,
	})
	if err != nil {
		emit(out, PhasePublish, "sync_publish_store", StatusWarning,
			"订阅已发布，但同步到 /publish 私有仓库失败", err.Error())
		return
	}

	if _, err := store.AddPublishRecord(publish.PublishRecordInput{
		WorkerConfigID: cfg.ID,
		WorkerName:     in.WorkerName,
		Hostname:       in.Hostname,
		BaseName:       in.BaseName,
		Version:        in.Version,
		FileName:       in.FileName,
		AccessURL:      in.AccessURL,
	}); err != nil {
		emit(out, PhasePublish, "sync_publish_store", StatusWarning,
			"订阅已发布，但同步到 /publish 发布记录失败", err.Error())
		return
	}

	emit(out, PhasePublish, "sync_publish_store", StatusOK,
		fmt.Sprintf("已同步到订阅分发：%s", in.FileName))
}
