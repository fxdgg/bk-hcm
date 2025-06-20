/*
 * TencentBlueKing is pleased to support the open source community by making
 * 蓝鲸智云 - 混合云管理平台 (BlueKing - Hybrid Cloud Management System) available.
 * Copyright (C) 2024 THL A29 Limited,
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

package actioncvm

import (
	"encoding/json"
	"fmt"
	"strings"

	actcli "hcm/cmd/task-server/logics/action/cli"
	actionflow "hcm/cmd/task-server/logics/flow"
	"hcm/pkg/api/core"
	corecvm "hcm/pkg/api/core/cloud/cvm"
	coretask "hcm/pkg/api/core/task"
	protocloud "hcm/pkg/api/data-service/cloud"
	hcprotocvm "hcm/pkg/api/hc-service/cvm"
	"hcm/pkg/criteria/constant"
	"hcm/pkg/dal/dao/tools"
	"hcm/pkg/kit"
	"hcm/pkg/logs"
	"hcm/pkg/thirdparty/api-gateway/cmdb"
	"hcm/pkg/tools/retry"
)

func (act BatchTaskCvmResetAction) resetTCloudCvm(kt *kit.Kit, detail coretask.Detail,
	req *hcprotocvm.TCloudBatchResetCvmReq) error {

	cvms, err := getCvmInfo(kt, req.CloudIDs)
	if err != nil {
		return err
	}

	if err = validateCvmSvrStatus(kt, cvms, detail); err != nil {
		logs.Errorf("fail to validate cvm status, err: %v, req: %+v, rid: %s")
		return err
	}

	var cloudErr error
	rangeMS := [2]uint{constant.CvmBatchTaskRetryDelayMinMS, constant.CvmBatchTaskRetryDelayMaxMS}
	policy := retry.NewRetryPolicy(0, rangeMS)
	for {
		cloudErr = actcli.GetHCService().TCloud.Cvm.ResetCvm(kt, req)
		cvmResetJson, jsonErr := json.Marshal(req)
		if jsonErr != nil {
			logs.Errorf("call hcservice api reset cvm json marshal, vendor: %s, detailID: %s, taskManageID: %s, "+
				"flowID: %s, cvmResetJson: %s, err: %+v, jsonErr: %+v, rid: %s", req.Vendor, detail.ID,
				detail.TaskManagementID, detail.FlowID, cvmResetJson, err, jsonErr, kt.Rid)
			return jsonErr
		}
		// 仅在碰到限频错误时进行重试
		if cloudErr != nil && strings.Contains(cloudErr.Error(), constant.TCloudLimitExceededErrCode) {
			if policy.RetryCount()+1 < actionflow.BatchTaskDefaultRetryTimes {
				// 	非最后一次重试，继续sleep
				logs.Errorf("call tcloud cvm reset reach rate limit, will sleep for retry, retry count: %d, "+
					"err: %v, rid: %s", policy.RetryCount(), cloudErr, kt.Rid)
				policy.Sleep()
				continue
			}
		}
		// 其他情况都跳过
		break
	}

	// 记录云端报错信息
	if cloudErr != nil {
		logs.Errorf("failed to call hcservice to reset cvm, err: %v, detailID: %s, taskManageID: %s, flowID: %s, "+
			"rid: %s", cloudErr, detail.ID, detail.TaskManagementID, detail.FlowID, kt.Rid)
		return cloudErr
	}

	return nil
}

func validateCvmSvrStatus(kt *kit.Kit, cvms []corecvm.Cvm[corecvm.TCloudCvmExtension],
	detail coretask.Detail) error {

	// get cvm info from cc, and check the srv_status isn't resetting
	mapBizToHostIDs := make(map[int64][]int64)
	for _, cvm := range cvms {
		mapBizToHostIDs[cvm.BkBizID] = append(mapBizToHostIDs[cvm.BkBizID], cvm.BkHostID)
	}
	for bizID, hostIDs := range mapBizToHostIDs {
		params := &cmdb.ListBizHostParams{
			BizID:  bizID,
			Fields: []string{"bk_host_id", "bk_host_innerip", "srv_status", "operator", "bk_bak_operator"},
			Page:   &cmdb.BasePage{Start: 0, Limit: int64(core.DefaultMaxPageLimit), Sort: "bk_host_id"},
			HostPropertyFilter: &cmdb.QueryFilter{
				Rule: &cmdb.CombinedRule{
					Condition: "AND",
					Rules: []cmdb.Rule{
						&cmdb.AtomRule{Field: "bk_host_id", Operator: "in", Value: hostIDs},
					},
				},
			},
		}
		hostResult, err := actcli.GetCMDBCli().ListBizHost(kt, params)
		if err != nil {
			logs.Errorf("request cmdb list biz host failed, err: %v, bizID: %d, hostIDs: %v, rid: %s",
				err, bizID, hostIDs, kt.Rid)
			return err
		}

		for _, host := range hostResult.Info {
			logs.Infof("cvm reset check status loop, hostID: %d, hostInnerIP: %s, operator: %s, bkOperator: %s, "+
				"detailCreator: %s, rid: %s", host.BkHostID, host.BkHostInnerIP, host.Operator, host.BkBakOperator,
				detail.Creator, kt.Rid)
			// 校验主备负责人
			if !strings.Contains(host.Operator, detail.Creator) &&
				!strings.Contains(host.BkBakOperator, detail.Creator) {

				logs.Errorf("cvm reset check operator failed, hostID: %d, 重装的主机负责人校验失败："+
					"主机[%s]的主要负责人[%s]、备份负责人[%s]和当前任务执行人[%s]不匹配，请重新校验后提交, rid: %s",
					host.BkHostID, host.BkHostInnerIP, host.Operator, host.BkBakOperator, detail.Creator, kt.Rid)
				return fmt.Errorf("重装的主机负责人校验失败：主机[%s]的负责人和当前任务执行人[%s]不匹配，请重新校验后提交",
					host.BkHostInnerIP, detail.Creator)
			}
		}
	}
	return nil
}

func getCvmInfo(kt *kit.Kit, cloudIDs []string) ([]corecvm.Cvm[corecvm.TCloudCvmExtension], error) {
	listReq := &protocloud.CvmListReq{
		Filter: tools.ContainersExpression("cloud_id", cloudIDs),
		Page:   core.NewDefaultBasePage(),
	}
	listResp, err := actcli.GetDataService().TCloud.Cvm.ListCvmExt(kt.Ctx, kt.Header(), listReq)
	if err != nil {
		logs.Errorf("request dataservice list tcloud cvm failed, err: %v, cloudID: %s, rid: %s",
			err, cloudIDs, kt.Rid)
		return nil, err
	}
	return listResp.Details, nil
}
