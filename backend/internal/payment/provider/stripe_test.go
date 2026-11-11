//go:build unit

package provider

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
	stripe "github.com/stripe/stripe-go/v85"
)

type stripeRefundBackend struct {
	params []*stripe.RefundCreateParams
}

func (b *stripeRefundBackend) Call(_ string, _ string, _ string, params stripe.ParamsContainer, v stripe.LastResponseSetter) error {
	refundParams, ok := params.(*stripe.RefundCreateParams)
	if !ok {
		return fmt.Errorf("unexpected params type %T", params)
	}
	b.params = append(b.params, refundParams)
	refund, ok := v.(*stripe.Refund)
	if !ok {
		return fmt.Errorf("unexpected response type %T", v)
	}
	refund.ID = "re_123"
	refund.Status = stripe.RefundStatusSucceeded
	return nil
}

func (*stripeRefundBackend) CallStreaming(string, string, string, stripe.ParamsContainer, stripe.StreamingLastResponseSetter) error {
	return nil
}

func (*stripeRefundBackend) CallRaw(string, string, string, []byte, *stripe.Params, stripe.LastResponseSetter) error {
	return nil
}

func (*stripeRefundBackend) CallMultipart(string, string, string, string, *bytes.Buffer, *stripe.Params, stripe.LastResponseSetter) error {
	return nil
}

func (*stripeRefundBackend) SetMaxNetworkRetries(int64) {}

func TestStripeRefundUsesStableAmountSpecificIdempotencyKey(t *testing.T) {
	backend := &stripeRefundBackend{}
	client := stripe.NewClient("sk_test", stripe.WithBackends(&stripe.Backends{API: backend}))
	provider := &Stripe{
		config:      map[string]string{"currency": "CNY"},
		initialized: true,
		sc:          client,
	}

	refund := func(amount string) {
		_, err := provider.Refund(context.Background(), payment.RefundRequest{
			TradeNo: "pi_123",
			OrderID: "sub2_order_456",
			Amount:  amount,
		})
		require.NoError(t, err)
	}

	refund("12.34")
	refund("12.34")
	refund("12.35")

	require.Len(t, backend.params, 3)
	require.Equal(t, int64(1234), *backend.params[0].Amount)
	require.Equal(t, "re-sub2_order_456-1234", *backend.params[0].IdempotencyKey)
	require.Equal(t, backend.params[0].IdempotencyKey, backend.params[1].IdempotencyKey)
	require.Equal(t, int64(1235), *backend.params[2].Amount)
	require.Equal(t, "re-sub2_order_456-1235", *backend.params[2].IdempotencyKey)
	require.NotEqual(t, *backend.params[0].IdempotencyKey, *backend.params[2].IdempotencyKey)
}

// V03-AC11：USD 订单 T1 退款 100，重复提交两次使用同一个幂等键 re-T1-10000。
func TestStripeRefundRepeatedSubmissionSharesIdempotencyKey(t *testing.T) {
	backend := &stripeRefundBackend{}
	client := stripe.NewClient("sk_test", stripe.WithBackends(&stripe.Backends{API: backend}))
	provider := &Stripe{
		config:      map[string]string{"currency": "USD"},
		initialized: true,
		sc:          client,
	}
	for i := 0; i < 2; i++ {
		_, err := provider.Refund(context.Background(), payment.RefundRequest{TradeNo: "pi_t1", OrderID: "T1", Amount: "100.00"})
		require.NoError(t, err)
	}
	require.Len(t, backend.params, 2)
	for _, p := range backend.params {
		require.NotNil(t, p.IdempotencyKey)
		require.Equal(t, "re-T1-10000", *p.IdempotencyKey)
		require.Equal(t, int64(10000), *p.Amount)
	}
}
