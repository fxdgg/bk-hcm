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

// Package vpc defines vpc service.
package vpc

import (
	"fmt"

	"hcm/pkg/adaptor/types"
	adcore "hcm/pkg/adaptor/types/core"
	"hcm/pkg/api/core"
	cloudcore "hcm/pkg/api/core/cloud"
	dataservice "hcm/pkg/api/data-service"
	"hcm/pkg/api/data-service/cloud"
	hcservice "hcm/pkg/api/hc-service"
	"hcm/pkg/criteria/enumor"
	"hcm/pkg/criteria/errf"
	"hcm/pkg/dal/dao/tools"
	daotypes "hcm/pkg/dal/dao/types"
	"hcm/pkg/logs"
	"hcm/pkg/rest"
	"hcm/pkg/runtime/filter"
	"hcm/pkg/tools/converter"
	"hcm/pkg/tools/uuid"
)

// TCloudVpcUpdate update tencent cloud vpc.
func (v vpc) TCloudVpcUpdate(cts *rest.Contexts) (interface{}, error) {
	id := cts.PathParameter("id").String()

	req := new(hcservice.VpcUpdateReq)
	if err := cts.DecodeInto(req); err != nil {
		return nil, errf.NewFromErr(errf.DecodeRequestFailed, err)
	}
	if err := req.Validate(); err != nil {
		return nil, errf.NewFromErr(errf.InvalidParameter, err)
	}

	getRes, err := v.cs.DataService().TCloud.Vpc.Get(cts.Kit.Ctx, cts.Kit.Header(), id)
	if err != nil {
		return nil, err
	}

	cli, err := v.ad.TCloud(cts.Kit, getRes.AccountID)
	if err != nil {
		return nil, err
	}

	updateOpt := new(types.TCloudVpcUpdateOption)
	err = cli.UpdateVpc(cts.Kit, updateOpt)
	if err != nil {
		return nil, err
	}

	updateReq := &cloud.VpcBatchUpdateReq[cloud.TCloudVpcUpdateExt]{
		Vpcs: []cloud.VpcUpdateReq[cloud.TCloudVpcUpdateExt]{{
			ID: id,
			VpcUpdateBaseInfo: cloud.VpcUpdateBaseInfo{
				Memo: req.Memo,
			},
		}},
	}
	err = v.cs.DataService().TCloud.Vpc.BatchUpdate(cts.Kit.Ctx, cts.Kit.Header(), updateReq)
	if err != nil {
		return nil, err
	}

	return nil, nil
}

// TCloudVpcDelete delete tencent cloud vpc.
func (v vpc) TCloudVpcDelete(cts *rest.Contexts) (interface{}, error) {
	id := cts.PathParameter("id").String()

	getRes, err := v.cs.DataService().TCloud.Vpc.Get(cts.Kit.Ctx, cts.Kit.Header(), id)
	if err != nil {
		return nil, err
	}

	cli, err := v.ad.TCloud(cts.Kit, getRes.AccountID)
	if err != nil {
		return nil, err
	}

	delOpt := &adcore.BaseRegionalDeleteOption{
		BaseDeleteOption: adcore.BaseDeleteOption{ResourceID: getRes.CloudID},
		Region:           getRes.Extension.Region,
	}
	err = cli.DeleteVpc(cts.Kit, delOpt)
	if err != nil {
		return nil, err
	}

	deleteReq := &dataservice.BatchDeleteReq{
		Filter: tools.EqualExpression("id", id),
	}
	err = v.cs.DataService().Global.Vpc.BatchDelete(cts.Kit.Ctx, cts.Kit.Header(), deleteReq)
	if err != nil {
		return nil, err
	}

	return nil, nil
}

// TCloudVpcSync sync tencent cloud vpc.
func (v vpc) TCloudVpcSync(cts *rest.Contexts) (interface{}, error) {
	req := new(hcservice.ResourceSyncReq)
	if err := cts.DecodeInto(req); err != nil {
		return nil, errf.NewFromErr(errf.DecodeRequestFailed, err)
	}
	if err := req.Validate(); err != nil {
		return nil, errf.NewFromErr(errf.InvalidParameter, err)
	}
	if len(req.Region) == 0 {
		return nil, errf.NewFromErr(errf.InvalidParameter, fmt.Errorf("region is required"))
	}

	var (
		vendorName = enumor.TCloud
		rsp        = hcservice.ResourceSyncResult{
			TaskID: uuid.UUID(),
		}
	)

	// batch get vpc list from cloudapi.
	list, err := v.BatchGetTCloudVpcList(cts, req)
	if err != nil || list == nil {
		logs.Errorf("[%s-vpc] request cloudapi response failed. accountID:%s, region:%s, err:%v",
			vendorName, req.AccountID, req.Region, err)
		return nil, err
	}

	// batch get vpc map from db.
	resourceDBMap, err := v.BatchGetVpcMapFromDB(cts, req, vendorName)
	if err != nil {
		logs.Errorf("[%s-vpc] batch get vpcdblist failed. accountID:%s, region:%s, err:%v",
			vendorName, req.AccountID, req.Region, err)
		return nil, err
	}

	// batch compare vendor vpc list.
	_, err = v.BatchCompareTcloudVpcList(cts, req, list, resourceDBMap)
	if err != nil {
		logs.Errorf("[%s-vpc] compare api and dblist failed. accountID:%s, region:%s, err:%v",
			vendorName, req.AccountID, req.Region, err)
		return nil, err
	}

	return rsp, nil
}

// BatchGetTCloudVpcList batch get vpc list from cloudapi.
func (v vpc) BatchGetTCloudVpcList(cts *rest.Contexts, req *hcservice.ResourceSyncReq) (
	*types.TCloudVpcListResult, error) {
	var (
		page  uint64
		count uint64 = adcore.TCloudQueryLimit
		list         = &types.TCloudVpcListResult{}
	)

	cli, err := v.ad.TCloud(cts.Kit, req.AccountID)
	if err != nil {
		return nil, err
	}

	for {
		offset := page * count
		opt := &adcore.TCloudListOption{
			Region: req.Region,
			Page: &adcore.TCloudPage{
				Offset: offset,
				Limit:  count,
			},
		}
		tmpList, tmpErr := cli.ListVpc(cts.Kit, opt)
		if tmpErr != nil || tmpList == nil {
			logs.Errorf("[%s-vpc]batch get cloudapi failed. accountID:%s, region:%s, offset:%d, count:%d, "+
				"err:%v", enumor.TCloud, req.AccountID, req.Region, offset, count, tmpErr)
			return nil, tmpErr
		}

		list.Details = append(list.Details, tmpList.Details...)
		if len(tmpList.Details) < int(count) {
			break
		}
		page++
	}
	return list, nil
}

// BatchGetVpcMapFromDB batch get vpc map from db.
func (v vpc) BatchGetVpcMapFromDB(cts *rest.Contexts, req *hcservice.ResourceSyncReq, vendor enumor.Vendor) (
	map[string]cloudcore.BaseVpc, error) {
	var (
		page        uint32
		count       = daotypes.DefaultMaxPageLimit
		resourceMap = map[string]cloudcore.BaseVpc{}
	)

	for {
		offset := page * uint32(count)
		expr := &filter.Expression{
			Op: filter.And,
			Rules: []filter.RuleFactory{
				&filter.AtomRule{
					Field: "vendor",
					Op:    filter.Equal.Factory(),
					Value: vendor,
				},
				&filter.AtomRule{
					Field: "account_id",
					Op:    filter.Equal.Factory(),
					Value: req.AccountID,
				},
			},
		}
		dbQueryReq := &core.ListReq{
			Filter: expr,
			Page:   &daotypes.BasePage{Count: false, Start: offset, Limit: count},
		}
		dbList, err := v.cs.DataService().Global.Vpc.List(cts.Kit.Ctx, cts.Kit.Header(), dbQueryReq)
		if err != nil {
			logs.Errorf("[%s-vpc]batch get vpclist db error. accountID:%s, region:%s, offset:%d, limit:%d, "+
				"err:%v", vendor, req.AccountID, req.Region, offset, count, err)
			return nil, err
		}
		if len(dbList.Details) == 0 {
			return resourceMap, nil
		}

		for _, item := range dbList.Details {
			resourceMap[item.CloudID] = item
		}
		if len(dbList.Details) < int(count) {
			break
		}
		page++
	}
	return resourceMap, nil
}

// BatchCompareTcloudVpcList batch compare vendor vpc list.
func (v vpc) BatchCompareTcloudVpcList(cts *rest.Contexts, req *hcservice.ResourceSyncReq,
	list *types.TCloudVpcListResult, resourceDBMap map[string]cloudcore.BaseVpc) (interface{}, error) {
	var (
		createResources []cloud.VpcCreateReq[cloud.TCloudVpcCreateExt]
		updateResources []cloud.VpcUpdateReq[cloud.TCloudVpcUpdateExt]
		existIDMap      = map[string]bool{}
		deleteIDs       []string
	)

	err := v.filterTcloudVpcList(req, list, resourceDBMap, &createResources, &updateResources, existIDMap)
	if err != nil {
		return nil, err
	}

	// update resource data
	if len(updateResources) > 0 {
		if err = v.cs.DataService().TCloud.Vpc.BatchUpdate(cts.Kit.Ctx, cts.Kit.Header(),
			&cloud.VpcBatchUpdateReq[cloud.TCloudVpcUpdateExt]{
				Vpcs: updateResources,
			}); err != nil {
			logs.Errorf("[%s-vpc]batch compare db update failed. accountID:%s, region:%s, err:%v",
				enumor.TCloud, req.AccountID, req.Region, err)
			return nil, err
		}
	}

	// add resource data
	if len(createResources) > 0 {
		if _, err = v.cs.DataService().TCloud.Vpc.BatchCreate(cts.Kit.Ctx, cts.Kit.Header(),
			&cloud.VpcBatchCreateReq[cloud.TCloudVpcCreateExt]{
				Vpcs: createResources,
			}); err != nil {
			logs.Errorf("[%s-vpc]batch compare db create failed. accountID:%s, region:%s, err:%v",
				enumor.TCloud, req.AccountID, req.Region, err)
			return nil, err
		}
	}

	// delete resource data
	for _, resourceItem := range resourceDBMap {
		if _, ok := existIDMap[resourceItem.ID]; !ok {
			deleteIDs = append(deleteIDs, resourceItem.ID)
		}
	}
	if len(deleteIDs) > 0 {
		if err = v.cs.DataService().Global.Vpc.BatchDelete(cts.Kit.Ctx, cts.Kit.Header(), &dataservice.BatchDeleteReq{
			Filter: tools.ContainersExpression("id", deleteIDs),
		}); err != nil {
			logs.Errorf("[%s-vpc]batch compare db delete failed. accountID:%s, region:%s, deleteIDs:%v, err:%v",
				enumor.TCloud, req.AccountID, req.Region, deleteIDs, err)
			return nil, err
		}
	}
	return nil, nil
}

func (v vpc) filterTcloudVpcList(req *hcservice.ResourceSyncReq, list *types.TCloudVpcListResult,
	resourceDBMap map[string]cloudcore.BaseVpc, createResources *[]cloud.VpcCreateReq[cloud.TCloudVpcCreateExt],
	updateResources *[]cloud.VpcUpdateReq[cloud.TCloudVpcUpdateExt], existIDMap map[string]bool) error {
	if list == nil || len(list.Details) == 0 {
		return fmt.Errorf("cloudapi vpclist is empty, accountID:%s, region:%s", req.AccountID, req.Region)
	}

	for _, item := range list.Details {
		// need compare and update resource data
		if resourceInfo, ok := resourceDBMap[item.CloudID]; ok {
			tmpRes := cloud.VpcUpdateReq[cloud.TCloudVpcUpdateExt]{
				ID: resourceInfo.ID,
				Extension: &cloud.TCloudVpcUpdateExt{
					IsDefault:       converter.ValToPtr(item.Extension.IsDefault),
					EnableMulticast: converter.ValToPtr(item.Extension.EnableMulticast),
					DnsServerSet:    item.Extension.DnsServerSet,
					DomainName:      converter.ValToPtr(item.Extension.DomainName),
				},
			}
			tmpRes.Name = converter.ValToPtr(item.Name)
			tmpRes.Memo = item.Memo

			if item.Extension.Cidr != nil {
				tmpCidrs := []cloud.TCloudCidr{}
				for _, cidrItem := range item.Extension.Cidr {
					tmpCidrs = append(tmpCidrs, cloud.TCloudCidr{
						Type:     cidrItem.Type,
						Cidr:     cidrItem.Cidr,
						Category: cidrItem.Category,
					})
				}
				tmpRes.Extension.Cidr = tmpCidrs
			}
			*updateResources = append(*updateResources, tmpRes)
			existIDMap[resourceInfo.ID] = true
		} else {
			// need add resource data
			tmpRes := cloud.VpcCreateReq[cloud.TCloudVpcCreateExt]{
				AccountID: req.AccountID,
				CloudID:   item.CloudID,
				Name:      converter.ValToPtr(item.Name),
				Category:  enumor.BizVpcCategory,
				Memo:      item.Memo,
				Extension: &cloud.TCloudVpcCreateExt{
					Region:          item.Extension.Region,
					IsDefault:       item.Extension.IsDefault,
					EnableMulticast: item.Extension.EnableMulticast,
					DnsServerSet:    item.Extension.DnsServerSet,
					DomainName:      item.Extension.DomainName,
				},
			}

			if item.Extension.Cidr != nil {
				tmpCidrs := []cloud.TCloudCidr{}
				for _, cidrItem := range item.Extension.Cidr {
					tmpCidrs = append(tmpCidrs, cloud.TCloudCidr{
						Type:     cidrItem.Type,
						Cidr:     cidrItem.Cidr,
						Category: cidrItem.Category,
					})
				}
				tmpRes.Extension.Cidr = tmpCidrs
			}
			*createResources = append(*createResources, tmpRes)
		}
	}
	return nil
}
