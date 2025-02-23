/*
Copyright 2021 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package usecase

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/apiserver/clients"
	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	v1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
	"github.com/oam-dev/kubevela/version"
)

// SystemInfoUsecase is usecase for systemInfoCollection
type SystemInfoUsecase interface {
	Get(ctx context.Context) (*model.SystemInfo, error)
	GetSystemInfo(ctx context.Context) (*v1.SystemInfoResponse, error)
	UpdateSystemInfo(ctx context.Context, sysInfo v1.SystemInfoRequest) (*v1.SystemInfoResponse, error)
	Init(ctx context.Context) error
}

type systemInfoUsecaseImpl struct {
	ds         datastore.DataStore
	kubeClient client.Client
}

// NewSystemInfoUsecase return a systemInfoCollectionUsecase
func NewSystemInfoUsecase(ds datastore.DataStore) SystemInfoUsecase {
	kubecli, err := clients.GetKubeClient()
	if err != nil {
		log.Logger.Fatalf("failed to get kube client: %s", err.Error())
	}
	return &systemInfoUsecaseImpl{ds: ds, kubeClient: kubecli}
}

func (u systemInfoUsecaseImpl) Get(ctx context.Context) (*model.SystemInfo, error) {
	// first get request will init systemInfoCollection{installId: {random}, enableCollection: true}
	info := &model.SystemInfo{}
	entities, err := u.ds.List(ctx, info, &datastore.ListOptions{})
	if err != nil {
		return nil, err
	}
	if len(entities) != 0 {
		info := entities[0].(*model.SystemInfo)
		if info.LoginType == "" {
			info.LoginType = model.LoginTypeLocal
		}
		return info, nil
	}
	installID := rand.String(16)
	info.InstallID = installID
	info.EnableCollection = true
	info.LoginType = model.LoginTypeLocal
	info.BaseModel = model.BaseModel{CreateTime: time.Now()}
	err = u.ds.Add(ctx, info)
	if err != nil {
		return nil, err
	}
	return info, nil
}

func (u systemInfoUsecaseImpl) GetSystemInfo(ctx context.Context) (*v1.SystemInfoResponse, error) {
	// first get request will init systemInfoCollection{installId: {random}, enableCollection: true}
	info, err := u.Get(ctx)
	if err != nil {
		return nil, err
	}
	return &v1.SystemInfoResponse{
		SystemInfo: convertInfoToBase(info),
		SystemVersion: v1.SystemVersion{
			VelaVersion: version.VelaVersion,
			GitVersion:  version.GitRevision,
		},
		StatisticInfo: v1.StatisticInfo{
			AppCount:                   info.StatisticInfo.AppCount,
			ClusterCount:               info.StatisticInfo.ClusterCount,
			EnableAddonList:            info.StatisticInfo.EnabledAddon,
			ComponentDefinitionTopList: info.StatisticInfo.TopKCompDef,
			TraitDefinitionTopList:     info.StatisticInfo.TopKTraitDef,
			WorkflowDefinitionTopList:  info.StatisticInfo.TopKWorkflowStepDef,
			PolicyDefinitionTopList:    info.StatisticInfo.TopKPolicyDef,
			UpdateTime:                 info.StatisticInfo.UpdateTime,
		},
	}, nil
}

func (u systemInfoUsecaseImpl) UpdateSystemInfo(ctx context.Context, sysInfo v1.SystemInfoRequest) (*v1.SystemInfoResponse, error) {
	info, err := u.Get(ctx)
	if err != nil {
		return nil, err
	}
	modifiedInfo := model.SystemInfo{
		InstallID:        info.InstallID,
		EnableCollection: sysInfo.EnableCollection,
		LoginType:        sysInfo.LoginType,
		BaseModel: model.BaseModel{
			CreateTime: info.CreateTime,
			UpdateTime: time.Now(),
		},
		StatisticInfo: info.StatisticInfo,
	}

	if sysInfo.LoginType == model.LoginTypeDex {
		admin := &model.User{Name: model.DefaultAdminUserName}
		if err := u.ds.Get(ctx, admin); err != nil {
			return nil, err
		}
		if admin.Email == "" {
			return nil, bcode.ErrEmptyAdminEmail
		}
		connectors, err := utils.GetDexConnectors(ctx, u.kubeClient)
		if err != nil {
			return nil, err
		}
		if len(connectors) < 1 {
			return nil, bcode.ErrNoDexConnector
		}
		if err := generateDexConfig(ctx, u.kubeClient, &model.UpdateDexConfig{
			VelaAddress: sysInfo.VelaAddress,
			Connectors:  connectors,
		}); err != nil {
			return nil, err
		}
	}
	err = u.ds.Put(ctx, &modifiedInfo)
	if err != nil {
		return nil, err
	}
	return &v1.SystemInfoResponse{
		SystemInfo: v1.SystemInfo{
			PlatformID:       modifiedInfo.InstallID,
			EnableCollection: modifiedInfo.EnableCollection,
			LoginType:        modifiedInfo.LoginType,
			// always use the initial createTime as system's installTime
			InstallTime: info.CreateTime,
		},
		SystemVersion: v1.SystemVersion{VelaVersion: version.VelaVersion, GitVersion: version.GitRevision},
	}, nil
}

func (u systemInfoUsecaseImpl) Init(ctx context.Context) error {
	info, err := u.Get(ctx)
	if err != nil {
		return err
	}
	signedKey = info.InstallID
	_, err = initDexConfig(ctx, u.kubeClient, "http://velaux.com")
	return err
}

func convertInfoToBase(info *model.SystemInfo) v1.SystemInfo {
	return v1.SystemInfo{
		PlatformID:       info.InstallID,
		EnableCollection: info.EnableCollection,
		LoginType:        info.LoginType,
		InstallTime:      info.CreateTime,
	}
}
