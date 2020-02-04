/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */
package cache

import (
	"github.com/apache/servicecomb-service-center/pkg/cache"
	"github.com/apache/servicecomb-service-center/pkg/log"
	"github.com/apache/servicecomb-service-center/pkg/util"
	"github.com/apache/servicecomb-service-center/server/core/backend"
	serviceUtil "github.com/apache/servicecomb-service-center/server/service/util"
	"golang.org/x/net/context"
)

type RevisionFilter struct {
	InstancesFilter
}

func (f *RevisionFilter) Name(ctx context.Context, parent *cache.Node) string {
	item := parent.Cache.Get(CACHE_FIND).(*VersionRuleCacheItem)
	requestRev := ctx.Value(CTX_FIND_REQUEST_REV).(string)
	if len(requestRev) == 0 || requestRev == item.Rev {
		return ""
	}
	return requestRev
}

func (f *RevisionFilter) Init(ctx context.Context, parent *cache.Node) (node *cache.Node, err error) {
	pCache := parent.Cache.Get(CACHE_FIND).(*VersionRuleCacheItem)
	requestRev := ctx.Value(CTX_FIND_REQUEST_REV).(string)
	// Use cache only and not call the backend directly when Indexer is not
	// creditable
	if len(requestRev) == 0 || requestRev == pCache.Rev ||
		!(backend.Store().Instance().Creditable()) {
		node = cache.NewNode()
		node.Cache.Set(CACHE_FIND, pCache)
		return
	}

	if pCache.BrokenWait() {
		node = cache.NewNode()
		node.Cache.Set(CACHE_FIND, pCache)
		return
	}

	cloneCtx := util.CloneContext(ctx)
	cloneCtx = util.SetContext(cloneCtx, serviceUtil.CTX_NOCACHE, "1")
	insts, _, err := f.Find(cloneCtx, parent)
	if err != nil {
		pCache.InitBrokenQueue()
		return nil, err
	}

	log.Warnf("the cache of finding instances api is broken, req[%s]!=cache[%s][%s]",
		requestRev, pCache.Rev, parent.Name)
	pCache.Instances = insts
	pCache.Broken()

	node = cache.NewNode()
	node.Cache.Set(CACHE_FIND, pCache)
	return
}
