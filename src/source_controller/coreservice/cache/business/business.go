/*
 * Tencent is pleased to support the open source community by making 蓝鲸 available.
 * Copyright (C) 2017-2018 THL A29 Limited, a Tencent company. All rights reserved.
 * Licensed under the MIT License (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 * http://opensource.org/licenses/MIT
 * Unless required by applicable law or agreed to in writing, software distributed under
 * the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
 * either express or implied. See the License for the specific language governing permissions and
 * limitations under the License.
 */

package business

import (
	"context"

	"configcenter/src/common"
	"configcenter/src/common/blog"
	"configcenter/src/common/mapstr"
	"configcenter/src/storage/dal"
	"configcenter/src/storage/reflector"
	"configcenter/src/storage/stream/types"
	"github.com/tidwall/gjson"
	"gopkg.in/redis.v5"
)

type business struct {
	key   keyGenerator
	rds   *redis.Client
	event reflector.Interface
	db    dal.DB
}

func (b *business) Run() error {
	page := 500
	opts := &types.ListWatchOptions{
		Options: types.Options{
			EventStruct: new(map[string]interface{}),
			Collection:  common.BKTableNameBaseApp,
		},
		PageSize: &page,
	}
	cap := &reflector.Capable{
		OnChange: reflector.OnChangeEvent{
			OnLister:     b.onUpsert,
			OnAdd:        b.onUpsert,
			OnUpdate:     b.onUpsert,
			OnListerDone: b.onListDone,
			OnDelete:     b.onDelete,
		},
	}

	return b.event.ListWatcher(context.Background(), opts, cap)
}

func (b *business) onUpsert(e *types.Event) {
	blog.V(4).Infof("received biz upsert event, oid: %s, operate: %s, doc: %s", e.Oid, e.OperationType, e.DocBytes)

	bizInfo := gjson.GetManyBytes(e.DocBytes, "bk_biz_id", "bk_biz_name")
	bizID := bizInfo[0].Int()
	bizName := bizInfo[1].String()
	if bizID == 0 {
		blog.Errorf("received biz upsert event, invalid biz id: %d, oid: %s", bizID, e.Oid)
		return
	}

	if len(bizName) == 0 {
		blog.Errorf("received biz upsert event, invalid biz name: %s, oid: %s", bizName, e.Oid)
		return
	}

	forUpsert := forUpsertCache{
		instID:            bizID,
		parentID:          0,
		name:              bizName,
		doc:               e.DocBytes,
		rds:               b.rds,
		listKey:           bizKey.listKeyWithBiz(bizID),
		listExpireKey:     bizKey.listExpireKeyWithBiz(bizID),
		detailKey:         bizKey.detailKey(bizID),
		detailExpireKey:   bizKey.detailExpireKey(bizID),
		parseListKeyValue: bizKey.parseListKeyValue,
		genListKeyValue:   bizKey.genListKeyValue,
		getInstName:       b.getBusinessName,
	}

	upsertListCache(&forUpsert)
}

func (b *business) onDelete(e *types.Event) {
	blog.V(4).Infof("received *unexpected* delete biz event, oid: %s, operate: %s, doc: %s", e.Oid, e.OperationType, e.DocBytes)
}

func (b *business) onListDone() {

}

func (b *business) getBusinessName(bizID int64) (string, error) {
	bizInfo := new(BizBaseInfo)
	filter := mapstr.MapStr{
		common.BKAppIDField: bizID,
	}
	if err := b.db.Table(common.BKTableNameBaseApp).Find(filter).One(context.Background(), bizInfo); err != nil {
		blog.Errorf("get biz %d name from mongo failed, err: %v", bizID, err)
		return "", err
	}
	return bizInfo.BusinessName, nil
}
