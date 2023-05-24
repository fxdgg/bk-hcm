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

package account

import (
	"fmt"

	accountsvc "hcm/cmd/cloud-server/service/account"
	"hcm/pkg/api/core"
	dataprotocloud "hcm/pkg/api/data-service/cloud"
	"hcm/pkg/client"
	"hcm/pkg/criteria/enumor"
	"hcm/pkg/rest"
	"hcm/pkg/runtime/filter"
	"hcm/pkg/tools/json"

	"github.com/TencentBlueKing/gopkg/conv"
)

// CheckReq 检查申请单的数据是否正确
func (a *ApplicationOfAddAccount) CheckReq() error {
	if err := a.req.Validate(); err != nil {
		return err
	}

	// 检查名称是否重复
	if err := a.isDuplicateName(a.req.Name); err != nil {
		return err
	}

	// 检查账号是否有效
	extensionJson, err := json.Marshal(a.req.Extension)
	if err != nil {
		return fmt.Errorf("json marshal extension failed, err: %w", err)
	}
	switch a.req.Vendor {
	case enumor.TCloud:
		_, err = accountsvc.ParseAndCheckTCloudExtension(a.Cts, a.Client, a.req.Type, extensionJson)
	case enumor.Aws:
		_, err = accountsvc.ParseAndCheckAwsExtension(a.Cts, a.Client, a.req.Type, extensionJson)
	case enumor.HuaWei:
		_, err = accountsvc.ParseAndCheckHuaWeiExtension(a.Cts, a.Client, a.req.Type, extensionJson)
	case enumor.Gcp:
		_, err = accountsvc.ParseAndCheckGcpExtension(a.Cts, a.Client, a.req.Type, extensionJson)
	case enumor.Azure:
		_, err = accountsvc.ParseAndCheckAzureExtension(a.Cts, a.Client, a.req.Type, extensionJson)
	default:
		err = fmt.Errorf("no support vendor: %s", a.req.Vendor)
	}
	if err != nil {
		return err
	}

	// 检查资源账号的主账号是否重复
	mainAccountIDField := vendorMainAccountIDFieldMap[a.req.Vendor]
	err = isDuplicateMainAccount(
		a.Cts, a.Client, a.req.Vendor, a.req.Type, mainAccountIDField, conv.ToString(a.req.Extension[mainAccountIDField]),
	)
	if err != nil {
		return err
	}

	return nil
}

func isDuplicateMainAccount(
	cts *rest.Contexts, client *client.ClientSet,
	vendor enumor.Vendor, accountType enumor.AccountType,
	mainAccountIDFieldName string, mainAccountIDFieldValue string,
) error {
	// 只需要检查资源账号或安全审计账号的主账号是否重复，其他类型账号不检查
	if accountType != enumor.ResourceAccount && accountType != enumor.SecurityAuditAccount {
		return nil
	}

	// TODO: 后续需要解决并发问题
	// 后台查询是否主账号重复
	result, err := client.DataService().Global.Account.List(
		cts.Kit.Ctx,
		cts.Kit.Header(),
		&dataprotocloud.AccountListReq{
			Filter: &filter.Expression{
				Op: filter.And,
				Rules: []filter.RuleFactory{
					filter.AtomRule{
						Field: "vendor",
						Op:    filter.Equal.Factory(),
						Value: string(vendor),
					},
					filter.AtomRule{
						Field: "type",
						Op:    filter.Equal.Factory(),
						Value: string(accountType),
					},
					filter.AtomRule{
						Field: fmt.Sprintf("extension.%s", mainAccountIDFieldName),
						Op:    filter.JSONEqual.Factory(),
						Value: mainAccountIDFieldValue,
					},
				},
			},
			Page: &core.BasePage{
				Count: true,
			},
		},
	)
	if err != nil {
		return err
	}

	if result.Count > 0 {
		return fmt.Errorf("%s[%s] should be not duplicate", mainAccountIDFieldName, mainAccountIDFieldValue)
	}

	return nil
}

func (a *ApplicationOfAddAccount) isDuplicateName(name string) error {
	// TODO: 后续需要解决并发问题
	// 后台查询是否主账号重复
	result, err := a.Client.DataService().Global.Account.List(
		a.Cts.Kit.Ctx,
		a.Cts.Kit.Header(),
		&dataprotocloud.AccountListReq{
			Filter: &filter.Expression{
				Op: filter.And,
				Rules: []filter.RuleFactory{
					filter.AtomRule{
						Field: "name",
						Op:    filter.Equal.Factory(),
						Value: name,
					},
				},
			},
			Page: &core.BasePage{
				Count: true,
			},
		},
	)
	if err != nil {
		return err
	}

	if result.Count > 0 {
		return fmt.Errorf("account name [%s] has already exits, should be not duplicate", name)
	}

	return nil
}
