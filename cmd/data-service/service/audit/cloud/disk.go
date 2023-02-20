/*
 * TencentBlueKing is pleased to support the open source community by making
 * 蓝鲸智云 - 混合云管理平台 (BlueKing - Hybrid Cloud Management System) available.
 * Copyright (C) 2022 THL A29 Limited,
 * a Tencent company. All rights reserved.
 * Licensed under the MIT License (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at http://opensource.org/licenses/MIT
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on
 * an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
 * either express or implied. See the License for the
 * specific language governing permissions and limitations under the License.
 *
 * We undertake not to change the open source license (MIT license) applicable
 *
 * to the current version of the project delivered to anyone in the future.
 */

package cloud

import (
	"hcm/pkg/api/core"
	protoaudit "hcm/pkg/api/data-service/audit"
	"hcm/pkg/criteria/enumor"
	"hcm/pkg/criteria/errf"
	diskdao "hcm/pkg/dal/dao/cloud/disk"
	"hcm/pkg/dal/dao/tools"
	"hcm/pkg/dal/dao/types"
	tableaudit "hcm/pkg/dal/table/audit"
	"hcm/pkg/dal/table/cloud/disk"
	"hcm/pkg/kit"
	"hcm/pkg/logs"
	"hcm/pkg/tools/converter"
)

func (ad Audit) diskAssignAuditBuild(kt *kit.Kit, assigns []protoaudit.CloudResourceAssignInfo) (
	[]*tableaudit.AuditTable, error,
) {
	ids := make([]string, 0, len(assigns))
	for _, one := range assigns {
		ids = append(ids, one.ResID)
	}
	idDiskMap, err := ad.listDisk(kt, ids)
	if err != nil {
		return nil, err
	}

	audits := make([]*tableaudit.AuditTable, 0, len(assigns))
	for _, one := range assigns {
		diskData, exist := idDiskMap[one.ResID]
		if !exist {
			continue
		}

		if one.AssignedResType != enumor.BizAuditAssignedResType {
			return nil, errf.New(errf.InvalidParameter, "assigned resource type is invalid")
		}
		changed := map[string]int64{"bk_biz_id": one.AssignedResID}

		audits = append(audits, &tableaudit.AuditTable{
			ResID:      one.ResID,
			CloudResID: diskData.CloudID,
			ResName:    converter.PtrToVal(&diskData.Name),
			ResType:    enumor.DiskAuditResType,
			Action:     enumor.Assign,
			BkBizID:    diskData.BkBizID,
			Vendor:     enumor.Vendor(diskData.Vendor),
			AccountID:  diskData.AccountID,
			Operator:   kt.User,
			Source:     kt.GetRequestSource(),
			Rid:        kt.Rid,
			AppCode:    kt.AppCode,
			Detail: &tableaudit.BasicDetail{
				Changed: changed,
			},
		})
	}

	return audits, nil
}

func (ad Audit) listDisk(kt *kit.Kit, ids []string) (map[string]*disk.DiskModel, error) {
	opt := &types.ListOption{
		Filter: tools.ContainersExpression("id", ids),
		Page:   core.DefaultBasePage,
	}
	list, err := ad.dao.GetObjectDao(disk.TableName).(*diskdao.DiskDao).List(kt, opt)
	if err != nil {
		logs.Errorf("list disk failed, err: %v, ids: %v, rid: %ad", err, ids, kt.Rid)
		return nil, err
	}

	result := make(map[string]*disk.DiskModel, len(list.Details))
	for _, one := range list.Details {
		result[one.ID] = one
	}

	return result, nil
}
