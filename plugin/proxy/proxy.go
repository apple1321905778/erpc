// Copyright 2017 HenryLee. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package proxy

import (
	"github.com/henrylee2cn/goutil"
	tp "github.com/henrylee2cn/teleport"
	"github.com/henrylee2cn/teleport/socket"
)

// A proxy plugin for handling unknown calling or pushing.

// Proxy creates a proxy plugin for handling unknown calling and pushing.
func Proxy(fn func(*ProxyLabel) Caller) tp.Plugin {
	return &proxy{
		callCaller: func(label *ProxyLabel) CallCaller {
			return fn(label)
		},
		pushCaller: func(label *ProxyLabel) PushCaller {
			return fn(label)
		},
	}
}

// ProxyCall creates a proxy plugin for handling unknown calling.
func ProxyCall(fn func(*ProxyLabel) CallCaller) tp.Plugin {
	return &proxy{callCaller: fn}
}

// ProxyPush creates a proxy plugin for handling unknown pushing.
func ProxyPush(fn func(*ProxyLabel) PushCaller) tp.Plugin {
	return &proxy{pushCaller: fn}
}

type (
	// Caller the object used to call and push
	Caller interface {
		CallCaller
		PushCaller
	}
	// CallCaller the object used to call
	CallCaller interface {
		Call(uri string, arg interface{}, result interface{}, setting ...socket.PacketSetting) tp.CallCmd
	}
	// PushCaller the object used to push
	PushCaller interface {
		Push(uri string, arg interface{}, setting ...socket.PacketSetting) *tp.Rerror
	}
	// ProxyLabel proxy label information
	ProxyLabel struct {
		SessionId, RealIp, Uri string
	}
	proxy struct {
		callCaller func(*ProxyLabel) CallCaller
		pushCaller func(*ProxyLabel) PushCaller
	}
)

var (
	_ tp.PostNewPeerPlugin = new(proxy)
)

func (p *proxy) Name() string {
	return "proxy"
}

func (p *proxy) PostNewPeer(peer tp.EarlyPeer) error {
	if p.callCaller != nil {
		peer.SetUnknownCall(p.call)
	}
	if p.pushCaller != nil {
		peer.SetUnknownPush(p.push)
	}
	return nil
}

func (p *proxy) call(ctx tp.UnknownCallCtx) (interface{}, *tp.Rerror) {
	var (
		label    ProxyLabel
		settings = make([]socket.PacketSetting, 1, 8)
	)
	label.SessionId = ctx.Session().Id()
	settings[0] = tp.WithSeq(label.SessionId + "@" + ctx.Seq())
	ctx.VisitMeta(func(key, value []byte) {
		settings = append(settings, tp.WithAddMeta(string(key), string(value)))
	})
	var (
		result      []byte
		realIpBytes = ctx.PeekMeta(tp.MetaRealIp)
	)
	if len(realIpBytes) == 0 {
		label.RealIp = ctx.Ip()
		settings = append(settings, tp.WithAddMeta(tp.MetaRealIp, label.RealIp))
	} else {
		label.RealIp = goutil.BytesToString(realIpBytes)
	}
	label.Uri = ctx.Uri()
	callcmd := p.callCaller(&label).Call(label.Uri, ctx.InputBodyBytes(), &result, settings...)
	callcmd.InputMeta().VisitAll(func(key, value []byte) {
		ctx.SetMeta(goutil.BytesToString(key), goutil.BytesToString(value))
	})
	rerr := callcmd.Rerror()
	if rerr != nil && rerr.Code < 200 && rerr.Code > 99 {
		rerr.Code = tp.CodeBadGateway
		rerr.Message = tp.CodeText(tp.CodeBadGateway)
	}
	return result, rerr
}

func (p *proxy) push(ctx tp.UnknownPushCtx) *tp.Rerror {
	var (
		label    ProxyLabel
		settings = make([]socket.PacketSetting, 1, 8)
	)
	label.SessionId = ctx.Session().Id()
	settings[0] = tp.WithSeq(label.SessionId + "@" + ctx.Seq())
	ctx.VisitMeta(func(key, value []byte) {
		settings = append(settings, tp.WithAddMeta(string(key), string(value)))
	})
	if realIpBytes := ctx.PeekMeta(tp.MetaRealIp); len(realIpBytes) == 0 {
		label.RealIp = ctx.Ip()
		settings = append(settings, tp.WithAddMeta(tp.MetaRealIp, label.RealIp))
	} else {
		label.RealIp = goutil.BytesToString(realIpBytes)
	}
	label.Uri = ctx.Uri()
	rerr := p.pushCaller(&label).Push(label.Uri, ctx.InputBodyBytes(), settings...)
	if rerr != nil && rerr.Code < 200 && rerr.Code > 99 {
		rerr.Code = tp.CodeBadGateway
		rerr.Message = tp.CodeText(tp.CodeBadGateway)
	}
	return rerr
}
