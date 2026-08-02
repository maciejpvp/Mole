package billing

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	stripe "github.com/stripe/stripe-go/v85"
	"github.com/stripe/stripe-go/v85/webhook"

	"mole-control-plane/internal/user"
)

var (
	ErrNotConfigured  = errors.New("billing is not configured")
	ErrNotFound       = errors.New("billing record not found")
	ErrOwnership      = errors.New("billing object does not belong to user")
	ErrInvalidStatus  = errors.New("setup intent is not complete")
	ErrInvalidWebhook = errors.New("invalid webhook")
)

type StripeGateway interface {
	CreateCustomer(context.Context, *stripe.CustomerCreateParams) (*stripe.Customer, error)
	CreateSetupIntent(context.Context, *stripe.SetupIntentCreateParams) (*stripe.SetupIntent, error)
	RetrieveSetupIntent(context.Context, string) (*stripe.SetupIntent, error)
}

type stripeGateway struct{ client *stripe.Client }

func (g stripeGateway) CreateCustomer(ctx context.Context, params *stripe.CustomerCreateParams) (*stripe.Customer, error) {
	return g.client.V1Customers.Create(ctx, params)
}

func (g stripeGateway) CreateSetupIntent(ctx context.Context, params *stripe.SetupIntentCreateParams) (*stripe.SetupIntent, error) {
	return g.client.V1SetupIntents.Create(ctx, params)
}

func (g stripeGateway) RetrieveSetupIntent(ctx context.Context, id string) (*stripe.SetupIntent, error) {
	return g.client.V1SetupIntents.Retrieve(ctx, id, nil)
}

type Service struct {
	db            *sql.DB
	stripe        StripeGateway
	webhookSecret string
	now           func() time.Time
}

type SetupIntentResult struct {
	SetupIntentID string `json:"setup_intent_id"`
	ClientSecret  string `json:"client_secret"`
	Status        string `json:"status"`
	Verified      bool   `json:"card_verified"`
}

type ConfirmationResult struct {
	SetupIntentID string `json:"setup_intent_id"`
	Status        string `json:"status"`
	Verified      bool   `json:"card_verified"`
	Plan          string `json:"plan"`
}

func NewService(db *sql.DB, secretKey, webhookSecret string) *Service {
	service := &Service{db: db, webhookSecret: strings.TrimSpace(webhookSecret), now: time.Now}
	if strings.TrimSpace(secretKey) != "" {
		service.stripe = stripeGateway{client: stripe.NewClient(strings.TrimSpace(secretKey))}
	}
	return service
}

func NewServiceWithGateway(db *sql.DB, gateway StripeGateway, webhookSecret string) *Service {
	return &Service{db: db, stripe: gateway, webhookSecret: strings.TrimSpace(webhookSecret), now: time.Now}
}

func (s *Service) CreateCardValidation(ctx context.Context, account user.User) (SetupIntentResult, error) {
	if s == nil || s.db == nil || s.stripe == nil {
		return SetupIntentResult{}, ErrNotConfigured
	}
	customerID, latestIntentID, verified, err := s.customerRecord(ctx, account)
	if err != nil {
		return SetupIntentResult{}, err
	}
	if verified {
		return SetupIntentResult{SetupIntentID: latestIntentID, Status: string(stripe.SetupIntentStatusSucceeded), Verified: true}, nil
	}
	if latestIntentID != "" {
		intent, retrieveErr := s.stripe.RetrieveSetupIntent(ctx, latestIntentID)
		if retrieveErr == nil && intent.Customer != nil && intent.Customer.ID == customerID && setupIntentIsReusable(intent.Status) && setupIntentIsCardOnly(intent) {
			return setupIntentResult(intent, false), nil
		}
	}
	params := &stripe.SetupIntentCreateParams{
		Customer:           stripe.String(customerID),
		PaymentMethodTypes: stripe.StringSlice([]string{"card"}),
		Usage:              stripe.String("off_session"),
		Metadata:           map[string]string{"mole_user_id": account.ID},
	}
	params.SetIdempotencyKey("mole-card-validation-v2-" + account.ID + "-" + fmt.Sprint(s.now().UTC().Unix()/3600))
	intent, err := s.stripe.CreateSetupIntent(ctx, params)
	if err != nil {
		return SetupIntentResult{}, fmt.Errorf("create Stripe SetupIntent: %w", err)
	}
	if intent.Customer == nil || intent.Customer.ID != customerID {
		return SetupIntentResult{}, ErrOwnership
	}
	if _, err := s.db.ExecContext(ctx, `
		UPDATE billing_customers
		SET latest_setup_intent_id = $1, updated_at = CURRENT_TIMESTAMP
		WHERE user_id = $2`, intent.ID, account.ID); err != nil {
		return SetupIntentResult{}, fmt.Errorf("store SetupIntent: %w", err)
	}
	return setupIntentResult(intent, false), nil
}

func (s *Service) ConfirmCardValidation(ctx context.Context, account user.User, intentID string) (ConfirmationResult, error) {
	if s == nil || s.db == nil || s.stripe == nil {
		return ConfirmationResult{}, ErrNotConfigured
	}
	intentID = strings.TrimSpace(intentID)
	if intentID == "" {
		return ConfirmationResult{}, ErrInvalidStatus
	}
	var customerID string
	if err := s.db.QueryRowContext(ctx, "SELECT stripe_customer_id FROM billing_customers WHERE user_id = $1", account.ID).Scan(&customerID); errors.Is(err, sql.ErrNoRows) {
		return ConfirmationResult{}, ErrNotFound
	} else if err != nil {
		return ConfirmationResult{}, fmt.Errorf("find billing customer: %w", err)
	}
	intent, err := s.stripe.RetrieveSetupIntent(ctx, intentID)
	if err != nil {
		return ConfirmationResult{}, fmt.Errorf("retrieve Stripe SetupIntent: %w", err)
	}
	if intent.Customer == nil || intent.Customer.ID != customerID {
		return ConfirmationResult{}, ErrOwnership
	}
	if intent.Status != stripe.SetupIntentStatusSucceeded {
		return ConfirmationResult{SetupIntentID: intent.ID, Status: string(intent.Status), Verified: false, Plan: account.Plan}, ErrInvalidStatus
	}
	if intent.PaymentMethod == nil || intent.PaymentMethod.ID == "" {
		return ConfirmationResult{}, ErrInvalidStatus
	}
	if err := s.activateFreeTier(ctx, account.ID, customerID, intent.ID, intent.PaymentMethod.ID); err != nil {
		return ConfirmationResult{}, err
	}
	return ConfirmationResult{SetupIntentID: intent.ID, Status: string(intent.Status), Verified: true, Plan: "free"}, nil
}

func (s *Service) HandleWebhook(ctx context.Context, payload []byte, signature string) error {
	if s == nil || s.db == nil || s.webhookSecret == "" {
		return ErrNotConfigured
	}
	event, err := webhook.ConstructEvent(payload, signature, s.webhookSecret)
	if err != nil {
		return fmt.Errorf("verify Stripe webhook: %w", err)
	}
	if event.Type != "setup_intent.succeeded" && event.Type != "setup_intent.setup_failed" && event.Type != "setup_intent.canceled" {
		return nil
	}
	var intent stripe.SetupIntent
	if err := json.Unmarshal(event.Data.Raw, &intent); err != nil {
		return fmt.Errorf("decode Stripe SetupIntent: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin Stripe webhook: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	var userID, customerID string
	err = tx.QueryRowContext(ctx, `
		SELECT user_id, stripe_customer_id
		FROM billing_customers
		WHERE latest_setup_intent_id = $1
		FOR UPDATE`, intent.ID).Scan(&userID, &customerID)
	if errors.Is(err, sql.ErrNoRows) {
		// The user may have started a newer validation attempt. The signed
		// event is valid but no longer represents the current billing state.
		return nil
	}
	if err != nil {
		return fmt.Errorf("find webhook billing record: %w", err)
	}
	if intent.Customer == nil || intent.Customer.ID != customerID {
		return ErrOwnership
	}
	var inserted bool
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO stripe_events (id, type) VALUES ($1, $2)
		ON CONFLICT (id) DO NOTHING
		RETURNING TRUE`, event.ID, event.Type).Scan(&inserted); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return tx.Commit()
		}
		return fmt.Errorf("record Stripe event: %w", err)
	}
	if inserted && event.Type == "setup_intent.succeeded" && intent.PaymentMethod != nil {
		if _, err := tx.ExecContext(ctx, `
			UPDATE billing_customers
			SET stripe_payment_method_id = $1, card_verified_at = $2, updated_at = CURRENT_TIMESTAMP
			WHERE user_id = $3`, intent.PaymentMethod.ID, s.now().UTC(), userID); err != nil {
			return fmt.Errorf("store verified payment method: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE users
			SET plan_id = (SELECT id FROM plans WHERE name = 'free')
			WHERE id = $1 AND plan_id = (SELECT id FROM plans WHERE name = 'pending')`, userID); err != nil {
			return fmt.Errorf("activate free plan: %w", err)
		}
	}
	return tx.Commit()
}

func (s *Service) customerRecord(ctx context.Context, account user.User) (string, string, bool, error) {
	var customerID, intentID string
	var verifiedAt sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT stripe_customer_id, COALESCE(latest_setup_intent_id, ''), card_verified_at
		FROM billing_customers WHERE user_id = $1`, account.ID).Scan(&customerID, &intentID, &verifiedAt)
	if errors.Is(err, sql.ErrNoRows) {
		params := &stripe.CustomerCreateParams{
			Email:    stripe.String(account.Email),
			Metadata: map[string]string{"mole_user_id": account.ID},
		}
		params.SetIdempotencyKey("mole-customer-" + account.ID)
		customer, createErr := s.stripe.CreateCustomer(ctx, params)
		if createErr != nil {
			return "", "", false, fmt.Errorf("create Stripe customer: %w", createErr)
		}
		if customer == nil || customer.ID == "" {
			return "", "", false, ErrNotFound
		}
		if _, insertErr := s.db.ExecContext(ctx, `
			INSERT INTO billing_customers (user_id, stripe_customer_id)
			VALUES ($1, $2)`, account.ID, customer.ID); insertErr != nil {
			return "", "", false, fmt.Errorf("store Stripe customer: %w", insertErr)
		}
		return customer.ID, "", false, nil
	}
	if err != nil {
		return "", "", false, fmt.Errorf("find Stripe customer: %w", err)
	}
	return customerID, intentID, verifiedAt.Valid, nil
}

func (s *Service) activateFreeTier(ctx context.Context, userID, customerID, intentID, paymentMethodID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin free-tier activation: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	var storedCustomer string
	if err := tx.QueryRowContext(ctx, "SELECT stripe_customer_id FROM billing_customers WHERE user_id = $1 FOR UPDATE", userID).Scan(&storedCustomer); err != nil {
		return fmt.Errorf("lock billing customer: %w", err)
	}
	if storedCustomer != customerID {
		return ErrOwnership
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE billing_customers
		SET stripe_payment_method_id = $1, latest_setup_intent_id = $2, card_verified_at = $3, updated_at = CURRENT_TIMESTAMP
		WHERE user_id = $4`, paymentMethodID, intentID, s.now().UTC(), userID); err != nil {
		return fmt.Errorf("store verified card: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE users SET plan_id = (SELECT id FROM plans WHERE name = 'free')
		WHERE id = $1 AND plan_id = (SELECT id FROM plans WHERE name = 'pending')`, userID); err != nil {
		return fmt.Errorf("activate free plan: %w", err)
	}
	return tx.Commit()
}

func setupIntentResult(intent *stripe.SetupIntent, verified bool) SetupIntentResult {
	result := SetupIntentResult{SetupIntentID: intent.ID, ClientSecret: intent.ClientSecret, Status: string(intent.Status), Verified: verified}
	if intent.Status == stripe.SetupIntentStatusSucceeded {
		result.Verified = true
	}
	return result
}

func setupIntentIsReusable(status stripe.SetupIntentStatus) bool {
	return status == stripe.SetupIntentStatusRequiresPaymentMethod || status == stripe.SetupIntentStatusRequiresConfirmation || status == stripe.SetupIntentStatusRequiresAction
}

func setupIntentIsCardOnly(intent *stripe.SetupIntent) bool {
	return intent != nil && len(intent.PaymentMethodTypes) == 1 && intent.PaymentMethodTypes[0] == "card"
}
