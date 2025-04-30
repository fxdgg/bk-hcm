/*
 * TencentBlueKing is pleased to support the open source community by making
 * 成本服务中心 (Cost Optimization Service Center) available.
 * Copyright (C) 2022 THL A29 Limited, a Tencent company. All rights reserved.
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

package apigateway

import (
	"encoding/json"
	"fmt"
	"net/http"

	"hcm/pkg/cc"
	"hcm/pkg/criteria/constant"
	"hcm/pkg/kit"
	"hcm/pkg/logs"
	"hcm/pkg/rest"
	"hcm/pkg/thirdparty/api-gateway/bkuser"
)

// BaseResponse is esb http base response.
type BaseResponse struct {
	Result  bool   `json:"result"`
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ApiGatewayResp ...
type ApiGatewayResp[T any] struct {
	Result         bool   `json:"result"`
	Code           int    `json:"code"`
	BKErrorCode    int    `json:"bk_error_code"`
	Message        string `json:"message"`
	BKErrorMessage string `json:"bk_error_msg"`
	Data           T      `json:"data"`
}

// ApiGatewayCall general call helper function for api gateway
func ApiGatewayCall[IT any, OT any](cli rest.ClientInterface, bkUserCli bkuser.Client, cfg *cc.ApiGateway,
	method rest.VerbType, kt *kit.Kit, req *IT, url string, urlParams ...any) (*OT, error) {

	header := GetCommonHeader(kt, bkUserCli, cfg)
	resp := new(ApiGatewayResp[*OT])
	err := cli.Verb(method).
		SubResourcef(url, urlParams...).
		WithContext(kt.Ctx).
		WithHeaders(header).
		Body(req).
		Do().Into(resp)

	if err != nil {
		logs.Errorf("fail to call api gateway api, err: %v, url: %s, rid: %s", err, url, kt.Rid)
		return nil, err
	}

	if !resp.Result || resp.Code != 0 {
		err := fmt.Errorf("failed to call api gateway, code: %d, msg: %s, bk_error_code: %d, bk_error_msg: %s",
			resp.Code, resp.Message, resp.BKErrorCode, resp.BKErrorMessage)
		logs.Errorf("api gateway returns error, url: %s, err: %v, rid: %s", url, err, kt.Rid)
		return nil, err
	}
	return resp.Data, nil
}

// ApiGatewayRespWithError ...
type ApiGatewayRespWithError[T any, E any] struct {
	Data  T `json:"data,omitempty"`
	Error E `json:"error,omitempty"`
}

// ApiGatewayCallWithRichError call helper function for api gateway that logs richer error details
// DT指定ApiGatewayResp中的Data dict具体结构，ET指定ApiGatewayRespWithError中的Error dict具体结构
func ApiGatewayCallWithRichError[IT any, DT any, ET any](cli rest.ClientInterface, cfg *cc.ApiGateway,
	method rest.VerbType, kt *kit.Kit, req *IT, url string, urlParams ...any) (ok *DT, neterr error, apierr *ET) {

	header := getCommonHeader(kt, cfg)
	resp := new(ApiGatewayRespWithError[*DT, *ET])

	// Into函数本身会将基本网络错误打印出日志
	err := cli.Verb(method).
		SubResourcef(url, urlParams...).
		WithContext(kt.Ctx).
		WithHeaders(header).
		Body(req).
		Do().Into(resp)

	if err != nil {
		logs.Errorf("fail to call api gateway api, err: %v, url: %s, rid: %s", err, url, kt.Rid)
		return nil, err, nil
	}

	if resp.Error != nil {
		errjson, _ := json.MarshalIndent(resp.Error, "", "    ")
		err := fmt.Errorf("failed to call api gateway: %s", string(errjson))
		logs.Errorf("api gateway returns error, url: %s, err: %v, rid: %s", url, err, kt.Rid)
		return nil, nil, resp.Error
	}

	return resp.Data, nil, nil
}

// ApiGatewayCallWithoutReq general call helper function for api gateway
func ApiGatewayCallWithoutReq[OT any](cli rest.ClientInterface, bkUserCli bkuser.Client, cfg *cc.ApiGateway,
	method rest.VerbType, kt *kit.Kit, params map[string]string, url string, urlParams ...any) (*OT, error) {

	header := GetCommonHeader(kt, bkUserCli, cfg)
	resp := new(ApiGatewayResp[*OT])
	err := cli.Verb(method).
		SubResourcef(url, urlParams...).
		WithContext(kt.Ctx).
		WithHeaders(header).
		WithParams(params).
		Do().Into(resp)

	if err != nil {
		logs.Errorf("fail to call api gateway api, err: %v, url: %s, rid: %s", err, url, kt.Rid)
		return nil, err
	}

	if !resp.Result || resp.Code != 0 {
		err := fmt.Errorf("failed to call api gateway, code: %d, msg: %s, bk_error_code: %d, bk_error_msg: %s",
			resp.Code, resp.Message, resp.BKErrorCode, resp.BKErrorMessage)
		logs.Errorf("api gateway returns error, url: %s, err: %v, rid: %s", url, err, kt.Rid)
		return nil, err
	}
	return resp.Data, nil
}

// GetCommonHeader get common header
func GetCommonHeader(kt *kit.Kit, bkUserCli bkuser.Client, cfg *cc.ApiGateway) http.Header {
	header := kt.Header()
	// 如果配置了指定用户，使用指定用户调用
	user := kt.User
	if len(cfg.User) > 0 {
		// TODO 多租户特有，单独提PR
		// 通过用户管理获取指定用户的bk_username
		// TODO 缓存
		username, err := getBkUsername(kt, bkUserCli, cfg.User)
		if err != nil {
			logs.Warnf("fail to get bk_username by user, err: %v, user: %s, rid: %s", err, user, kt.Rid)
			return header
		}
		user = username
	}
	// TODO: 目前调用方式和itsm 不同，后期改成统一的ApiGateWay 客户端
	bkAuth := fmt.Sprintf(`{"bk_app_code": "%s", "bk_app_secret": "%s","bk_username":"%s"}`,
		cfg.AppCode, cfg.AppSecret, user)
	header.Set(constant.BKGWAuthKey, bkAuth)
	header.Set(constant.RidKey, kt.Rid)
	header.Set(constant.TenantIDKey, kt.TenantID)
	return header
}

// GetCommonHeaderWithoutUser get common header without bk_username
func GetCommonHeaderWithoutUser(kt *kit.Kit, cfg *cc.ApiGateway) http.Header {
	header := kt.Header()
	bkAuth := fmt.Sprintf(`{"bk_app_code": "%s", "bk_app_secret": "%s"}`, cfg.AppCode, cfg.AppSecret)
	header.Set(constant.BKGWAuthKey, bkAuth)
	header.Set(constant.RidKey, kt.Rid)
	return header
}

// getBkUsername get bk_username by login_name
func getBkUsername(kt *kit.Kit, bkUserCli bkuser.Client, loginName string) (string, error) {
	resp, err := bkUserCli.BatchLookupVirtualUser(kt, []string{loginName}, "login_name")
	if err != nil {
		logs.Errorf("fail to get bk_username by login_name, err: %v, login_name: %s, rid: %s", err, loginName,
			kt.Rid)
		return "", err
	}

	var bkUsername string
	for _, item := range resp.Data {
		if item.LoginName == loginName {
			bkUsername = item.BkUsername
			break
		}
	}

	if bkUsername == "" {
		logs.Errorf("login_name not found bk_username, login_name: %s, rid: %s", loginName, kt.Rid)
		return "", fmt.Errorf("login_name not found bk_username, login_name: %s", loginName)
	}
	return bkUsername, nil
}
