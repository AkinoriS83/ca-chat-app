package usecase

import (
	"context"

	"github.com/AkinoriS83/ca-chat-app/server/internal/application/port"
	"github.com/AkinoriS83/ca-chat-app/server/internal/domain"
)

// peerResolver は、相手の指定を扱う手順を、まとめる。相手の特定、履歴、送信の3つが使う。
// 3つの操作に、同じ数え方と、同じ拒否の順を適用するためである(RL-31)。
type peerResolver struct {
	users   port.UserDirectory
	reader  port.ConversationReader
	limiter port.AttemptLimiter
	rules   domain.LimitRules
	clock   port.Clock
}

// resolvedPeer は、特定した相手である。
type resolvedPeer struct {
	user         domain.User
	pair         domain.DirectPair
	conversation domain.Conversation // established が真の場合だけ
	established  bool                // 自分との間に、成立した会話があるか
}

// resolve は、確認済みの利用者(actor)と、相手の指定を受けて、相手を特定する。
// 拒否は、次の順に判定する。識別名の規則 → 自分自身 → 上限 → 相手が居ない。
// invalidHandle は、相手の指定が、識別名の規則を満たさない場合の、拒否の種類である
// (相手の特定では InvalidInput、履歴と送信では NotFound)。
func (r *peerResolver) resolve(ctx context.Context, a *attempt, actor domain.User, rawPeer string, invalidHandle Rejection) (resolvedPeer, error) {
	handle, err := domain.NewHandle(rawPeer)
	if err != nil {
		return resolvedPeer{}, rejected(invalidHandle)
	}
	// 自分自身かどうかは、大文字と小文字を区別せずに、判定する。
	if handle.Key() == actor.Handle.Key() {
		return resolvedPeer{}, rejected(InvalidPeer)
	}

	peer := resolvedPeer{}
	user, found, err := r.users.FindByHandle(ctx, handle)
	if err != nil {
		return resolvedPeer{}, err
	}
	if found {
		peer.user = user
		peer.pair, err = domain.NewDirectPair(actor.ID, user.ID)
		if err != nil {
			return resolvedPeer{}, rejected(InvalidPeer)
		}
		peer.conversation, peer.established, err = r.reader.FindDirect(ctx, peer.pair)
		if err != nil {
			return resolvedPeer{}, err
		}
	}

	// 成立した会話がない相手を指定した要求を、数える。相手が居るかどうかに、依らない。
	// 相手が居るかどうかを、応答に出す前に行う。上限に達している間、居る場合と、居ない場合が、同じ応答になる。
	// 成立した会話がある相手は、居ることを、既に知っている相手であり、数えない(RL-D5)。
	if !peer.established {
		exceeded, admitted, err := r.limiter.Admit(ctx, r.clock.Now(), r.rules.PeerLookupByUser(actor.ID))
		if err != nil {
			return resolvedPeer{}, err
		}
		if !admitted {
			a.limitExceeded(ctx, exceeded)
			return resolvedPeer{}, rejected(RateLimited)
		}
	}

	if !found {
		return resolvedPeer{}, rejected(NotFound)
	}
	return peer, nil
}

// PeerFinder は、「相手を特定する」ユースケースである(UC-08)。
type PeerFinder struct {
	gate     *SessionGate
	resolver *peerResolver
	events   port.EventRecorder
}

// NewPeerFinder は、PeerFinder を作る。
func NewPeerFinder(gate *SessionGate, users port.UserDirectory, reader port.ConversationReader, limiter port.AttemptLimiter, rules domain.LimitRules, clock port.Clock, events port.EventRecorder) *PeerFinder {
	return &PeerFinder{
		gate:     gate,
		resolver: &peerResolver{users: users, reader: reader, limiter: limiter, rules: rules, clock: clock},
		events:   events,
	}
}

// Find は、識別名が一致する利用者の、識別名(登録した通り)と表示名を返す。会話は、開かない。
// 一致する利用者が居ない場合は、NotFound で拒否する。候補は、示さない(R-2、AC-7)。
func (f *PeerFinder) Find(ctx context.Context, in FindPeerInput) (out FindPeerOutput, err error) {
	a := begin(f.events, "find peer", in.Source)
	defer a.finish(ctx, &err)

	_, actor, err := f.gate.verify(ctx, a, in.Actor)
	if err != nil {
		return FindPeerOutput{}, err
	}
	peer, err := f.resolver.resolve(ctx, a, actor, in.Handle, InvalidInput)
	if err != nil {
		return FindPeerOutput{}, err
	}
	return FindPeerOutput{Peer: toUserView(peer.user)}, nil
}
