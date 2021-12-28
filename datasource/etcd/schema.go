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

package etcd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/apache/servicecomb-service-center/datasource"
	"github.com/apache/servicecomb-service-center/datasource/etcd/path"
	"github.com/apache/servicecomb-service-center/datasource/etcd/sd"
	serviceUtil "github.com/apache/servicecomb-service-center/datasource/etcd/util"
	"github.com/apache/servicecomb-service-center/datasource/schema"
	"github.com/apache/servicecomb-service-center/pkg/log"
	"github.com/apache/servicecomb-service-center/pkg/util"
	mapset "github.com/deckarep/golang-set"
	"github.com/go-chassis/cari/discovery"
	"github.com/little-cui/etcdadpt"
)

func init() {
	schema.Install("etcd", NewSchemaDAO)
	schema.Install("embeded_etcd", NewSchemaDAO)
	schema.Install("embedded_etcd", NewSchemaDAO)
}

func NewSchemaDAO(opts schema.Options) (schema.DAO, error) {
	return &SchemaDAO{}, nil
}

type SchemaDAO struct{}

func (dao *SchemaDAO) GetRef(ctx context.Context, refRequest *schema.RefRequest) (*schema.Ref, error) {
	domainProject := util.ParseDomainProject(ctx)
	domain := util.ParseDomain(ctx)
	project := util.ParseProject(ctx)
	serviceID := refRequest.ServiceID
	schemaID := refRequest.SchemaID

	refKey := path.GenerateServiceSchemaRefKey(domainProject, serviceID, schemaID)
	refResp, err := sd.SchemaRef().Search(ctx, serviceUtil.ContextOptions(ctx, etcdadpt.WithStrKey(refKey))...)
	if err != nil {
		log.Error(fmt.Sprintf("get service[%s] schema-ref[%s] failed", serviceID, schemaID), err)
		return nil, err
	}
	if len(refResp.Kvs) == 0 {
		return nil, schema.ErrSchemaNotFound
	}

	summary, err := getSummary(ctx, serviceID, schemaID)
	if err != nil {
		log.Error(fmt.Sprintf("get service[%s] schema-summary[%s] failed", serviceID, schemaID), err)
		return nil, err
	}

	return &schema.Ref{
		Domain:    domain,
		Project:   project,
		ServiceID: serviceID,
		SchemaID:  schemaID,
		Hash:      refResp.Kvs[0].Value.(string),
		Summary:   summary,
	}, nil
}

func getSummary(ctx context.Context, serviceID string, schemaID string) (string, error) {
	domainProject := util.ParseDomainProject(ctx)
	summaryKey := path.GenerateServiceSchemaSummaryKey(domainProject, serviceID, schemaID)
	summaryResp, err := sd.SchemaSummary().Search(ctx, serviceUtil.ContextOptions(ctx, etcdadpt.WithStrKey(summaryKey))...)
	if err != nil {
		return "", err
	}
	var summary string
	if len(summaryResp.Kvs) > 0 {
		summary = summaryResp.Kvs[0].Value.(string)
	}
	return summary, nil
}

func (dao *SchemaDAO) ListRef(ctx context.Context, refRequest *schema.RefRequest) ([]*schema.Ref, error) {
	domainProject := util.ParseDomainProject(ctx)
	domain := util.ParseDomain(ctx)
	project := util.ParseProject(ctx)
	serviceID := refRequest.ServiceID

	refPrefixKey := path.GenerateServiceSchemaRefKey(domainProject, serviceID, "")
	refResp, err := sd.SchemaRef().Search(ctx, serviceUtil.ContextOptions(ctx,
		etcdadpt.WithStrKey(refPrefixKey), etcdadpt.WithPrefix())...)
	if err != nil {
		log.Error(fmt.Sprintf("get service[%s] schema-refs failed", serviceID), err)
		return nil, err
	}

	summaryMap, err := getSummaryMap(ctx, serviceID)
	if err != nil {
		log.Error(fmt.Sprintf("get service[%s] schema-summaries failed", serviceID), err)
		return nil, err
	}

	schemas := make([]*schema.Ref, 0, len(refResp.Kvs))
	for _, kv := range refResp.Kvs {
		_, _, schemaID := path.GetInfoFromSchemaRefKV(kv.Key)
		schemas = append(schemas, &schema.Ref{
			Domain:    domain,
			Project:   project,
			ServiceID: serviceID,
			SchemaID:  schemaID,
			Hash:      kv.Value.(string),
			Summary:   summaryMap[schemaID], // may be empty
		})
	}
	return schemas, nil
}

func getSummaryMap(ctx context.Context, serviceID string) (map[string]string, error) {
	domainProject := util.ParseDomainProject(ctx)
	summaryPrefixKey := path.GenerateServiceSchemaSummaryKey(domainProject, serviceID, "")
	summaryResp, err := sd.SchemaSummary().Search(ctx, serviceUtil.ContextOptions(ctx,
		etcdadpt.WithStrKey(summaryPrefixKey), etcdadpt.WithPrefix())...)
	if err != nil {
		return nil, err
	}

	summaryMap := make(map[string]string, len(summaryResp.Kvs))
	for _, kv := range summaryResp.Kvs {
		_, _, schemaID := path.GetInfoFromSchemaSummaryKV(kv.Key)
		summaryMap[schemaID] = kv.Value.(string)
	}
	return summaryMap, nil
}

func (dao *SchemaDAO) DeleteRef(ctx context.Context, refRequest *schema.RefRequest) error {
	domainProject := util.ParseDomainProject(ctx)
	serviceID := refRequest.ServiceID
	schemaID := refRequest.SchemaID
	refKey := path.GenerateServiceSchemaRefKey(domainProject, serviceID, schemaID)
	summaryKey := path.GenerateServiceSchemaSummaryKey(domainProject, serviceID, schemaID)
	options := []etcdadpt.OpOptions{
		etcdadpt.OpDel(etcdadpt.WithStrKey(refKey)),
		etcdadpt.OpDel(etcdadpt.WithStrKey(summaryKey)),
	}
	cmp, err := etcdadpt.TxnWithCmp(ctx, options, etcdadpt.If(etcdadpt.ExistKey(refKey)), options)
	if err != nil {
		log.Error(fmt.Sprintf("delete service[%s] schema-ref[%s] failed", serviceID, schemaID), err)
		return discovery.NewError(discovery.ErrUnavailableBackend, err.Error())
	}
	if !cmp.Succeeded {
		log.Error(fmt.Sprintf("service[%s] schema-ref[%s] does not exist", serviceID, schemaID), nil)
		return schema.ErrSchemaNotFound
	}
	return nil
}

func (dao *SchemaDAO) GetContent(ctx context.Context, contentRequest *schema.ContentRequest) (*schema.Content, error) {
	domainProject := util.ParseDomainProject(ctx)
	domain := util.ParseDomain(ctx)
	project := util.ParseProject(ctx)
	hash := contentRequest.Hash

	contentKey := path.GenerateServiceSchemaContentKey(domainProject, hash)
	kv, err := etcdadpt.Get(ctx, contentKey)
	if err != nil {
		log.Error(fmt.Sprintf("get schema content[%s] failed", hash), err)
		return nil, err
	}
	if kv == nil {
		return nil, schema.ErrSchemaContentNotFound
	}

	return &schema.Content{
		Domain:  domain,
		Project: project,
		Hash:    hash,
		Content: string(kv.Value),
	}, nil
}

func (dao *SchemaDAO) PutContent(ctx context.Context, contentRequest *schema.PutContentRequest) error {
	domainProject := util.ParseDomainProject(ctx)
	schemaID := contentRequest.SchemaID
	serviceID := contentRequest.ServiceID
	content := contentRequest.Content

	service, err := datasource.GetMetadataManager().GetService(ctx, &discovery.GetServiceRequest{
		ServiceId: serviceID,
	})
	if err != nil {
		log.Error(fmt.Sprintf("get service[%s] failed", serviceID), err)
		return err
	}

	refKey := path.GenerateServiceSchemaRefKey(domainProject, serviceID, schemaID)
	contentKey := path.GenerateServiceSchemaContentKey(domainProject, content.Hash)
	summaryKey := path.GenerateServiceSchemaSummaryKey(domainProject, serviceID, schemaID)
	existContentOptions := []etcdadpt.OpOptions{
		etcdadpt.OpPut(etcdadpt.WithStrKey(refKey), etcdadpt.WithStrValue(content.Hash)),
		etcdadpt.OpPut(etcdadpt.WithStrKey(summaryKey), etcdadpt.WithStrValue(content.Summary)),
	}

	// append the schemaID into service.Schemas if schemaID is new
	if !util.SliceHave(service.Schemas, schemaID) {
		service.Schemas = append(service.Schemas, schemaID)
		body, err := json.Marshal(service)
		if err != nil {
			log.Error("marshal service failed", err)
			return err
		}
		serviceKey := path.GenerateServiceKey(domainProject, serviceID)
		existContentOptions = append(existContentOptions,
			etcdadpt.OpPut(etcdadpt.WithStrKey(serviceKey), etcdadpt.WithValue(body)))
	}

	newContentOptions := append(existContentOptions,
		etcdadpt.OpPut(etcdadpt.WithStrKey(contentKey), etcdadpt.WithStrValue(content.Content)),
	)
	cmp, err := etcdadpt.TxnWithCmp(ctx, newContentOptions, etcdadpt.If(etcdadpt.NotExistKey(contentKey)), existContentOptions)
	if err != nil {
		log.Error(fmt.Sprintf("put kv[%s] failed", refKey), err)
		return err
	}
	if cmp.Succeeded {
		log.Info(fmt.Sprintf("put kv[%s] and content[chars: %d]", refKey, len(content.Content)))
	} else {
		log.Info(fmt.Sprintf("put kv[%s] without content", refKey))
	}
	return nil
}

func (dao *SchemaDAO) PutManyContent(ctx context.Context, contentRequest *schema.PutManyContentRequest) error {
	domainProject := util.ParseDomainProject(ctx)
	serviceID := contentRequest.ServiceID

	if len(contentRequest.SchemaIDs) != len(contentRequest.Contents) {
		log.Error(fmt.Sprintf("service[%s] contents request invalid", serviceID), nil)
		return discovery.NewError(discovery.ErrInvalidParams, "contents request invalid")
	}

	service, err := datasource.GetMetadataManager().GetService(ctx, &discovery.GetServiceRequest{
		ServiceId: serviceID,
	})
	if err != nil {
		log.Error(fmt.Sprintf("get service[%s] failed", serviceID), err)
		return err
	}

	// unsafe!
	schemaIDs, options := transformSchemaIDsAndOptions(domainProject, serviceID, service.Schemas, contentRequest)

	// should update service.Schemas
	service.Schemas = schemaIDs
	body, err := json.Marshal(service)
	if err != nil {
		log.Error("marshal service failed", err)
		return err
	}
	serviceKey := path.GenerateServiceKey(domainProject, serviceID)
	options = append(options, etcdadpt.OpPut(etcdadpt.WithStrKey(serviceKey), etcdadpt.WithValue(body)))
	return etcdadpt.Txn(ctx, options)
}

func transformSchemaIDsAndOptions(domainProject string, serviceID string, oldSchemaIDs []string, contentRequest *schema.PutManyContentRequest) ([]string, []etcdadpt.OpOptions) {
	pendingDeleteSchemaIDs := mapset.NewSet()
	for _, schemaID := range oldSchemaIDs {
		pendingDeleteSchemaIDs.Add(schemaID)
	}

	var options []etcdadpt.OpOptions
	schemaIDs := make([]string, 0, len(contentRequest.Contents))
	for i, content := range contentRequest.Contents {
		schemaID := contentRequest.SchemaIDs[i]
		refKey := path.GenerateServiceSchemaRefKey(domainProject, serviceID, schemaID)
		contentKey := path.GenerateServiceSchemaContentKey(domainProject, content.Hash)
		summaryKey := path.GenerateServiceSchemaSummaryKey(domainProject, serviceID, schemaID)
		options = append(options,
			etcdadpt.OpPut(etcdadpt.WithStrKey(refKey), etcdadpt.WithStrValue(content.Hash)),
			etcdadpt.OpPut(etcdadpt.WithStrKey(contentKey), etcdadpt.WithStrValue(content.Content)),
			etcdadpt.OpPut(etcdadpt.WithStrKey(summaryKey), etcdadpt.WithStrValue(content.Summary)),
		)
		schemaIDs = append(schemaIDs, schemaID)
		pendingDeleteSchemaIDs.Remove(schemaID)
	}

	for item := range pendingDeleteSchemaIDs.Iter() {
		schemaID := item.(string)
		refKey := path.GenerateServiceSchemaRefKey(domainProject, serviceID, schemaID)
		summaryKey := path.GenerateServiceSchemaSummaryKey(domainProject, serviceID, schemaID)
		options = append(options,
			etcdadpt.OpDel(etcdadpt.WithStrKey(refKey)),
			etcdadpt.OpDel(etcdadpt.WithStrKey(summaryKey)),
		)
	}
	return schemaIDs, options
}

func (dao *SchemaDAO) DeleteContent(ctx context.Context, contentRequest *schema.ContentRequest) error {
	domainProject := util.ParseDomainProject(ctx)
	hash := contentRequest.Hash

	// TODO bad performance
	hashMap, err := getContentHashMap(ctx)
	if err != nil {
		log.Error(fmt.Sprintf("get schema[%s] refs map failed", hash), err)
		return err
	}
	if _, ok := hashMap[hash]; ok {
		log.Error(fmt.Sprintf("schema[%s] is reference by service", hash), nil)
		return discovery.NewError(discovery.ErrInvalidParams, "Schema has reference.")
	}

	contentKey := path.GenerateServiceSchemaContentKey(domainProject, hash)
	success, err := etcdadpt.Delete(ctx, contentKey)
	if err != nil {
		log.Error(fmt.Sprintf("delete schema content[%s] failed", hash), err)
		return err
	}
	if !success {
		log.Error(fmt.Sprintf("delete schema content[%s] failed", hash), schema.ErrSchemaContentNotFound)
		return schema.ErrSchemaContentNotFound
	}
	log.Info(fmt.Sprintf("delete schema content[%s]", hash))
	return nil
}

func getContentHashMap(ctx context.Context) (map[string]struct{}, error) {
	domainProject := util.ParseDomainProject(ctx)
	refPrefixKey := path.GetServiceSchemaRefRootKey(domainProject) + path.SPLIT
	refResp, err := sd.SchemaRef().Search(ctx, serviceUtil.ContextOptions(ctx,
		etcdadpt.WithStrKey(refPrefixKey), etcdadpt.WithPrefix())...)
	if err != nil {
		return nil, err
	}
	refMap := make(map[string]struct{})
	for _, kv := range refResp.Kvs {
		refMap[kv.Value.(string)] = struct{}{}
	}
	return refMap, nil
}

func (dao *SchemaDAO) ListHash(ctx context.Context) ([]*schema.Content, error) {
	panic("implement me")
}

func (dao *SchemaDAO) ExistRef(ctx context.Context, hash *schema.ContentRequest) (*schema.Ref, error) {
	panic("implement me")
}