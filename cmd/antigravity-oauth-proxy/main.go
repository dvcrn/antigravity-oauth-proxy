package main

import (
	"fmt"

	"github.com/dvcrn/antigravity-oauth-proxy/internal/antigravity"
	"github.com/dvcrn/antigravity-oauth-proxy/internal/credentials"
	"github.com/dvcrn/antigravity-oauth-proxy/internal/env"
	"github.com/dvcrn/antigravity-oauth-proxy/internal/logger"
	"github.com/dvcrn/antigravity-oauth-proxy/internal/project"
	"github.com/dvcrn/antigravity-oauth-proxy/internal/server"
)

func main() {
	port := env.GetOrDefault("PORT", "9878")

	// Create file provider
	provider, err := credentials.NewFileProvider()
	if err != nil {
		logger.Get().Fatal().Err(err).Msg("Failed to create credentials provider")
	}

	// Perform startup auth check
	logger.Get().Info().Msg("Performing startup authentication check...")
	antigravityClient := antigravity.NewClient(provider)
	loadAssistResponse, err := antigravityClient.LoadCodeAssist()
	if err != nil {
		logger.Get().Warn().Err(err).Msg("Startup authentication check failed.")
	} else {
		tier := fmt.Sprintf("%s (%s)", loadAssistResponse.CurrentTier.Name, loadAssistResponse.CurrentTier.ID)
		logger.Get().Info().
			Str("tier", tier).
			Str("tier_id", loadAssistResponse.CurrentTier.ID).
			Str("tier_name", loadAssistResponse.CurrentTier.Name).
			Str("tier_description", loadAssistResponse.CurrentTier.Description).
			Int("allowed_tiers", len(loadAssistResponse.AllowedTiers)).
			Str("project_id", loadAssistResponse.CloudAICompanionProject).
			Bool("gcp_managed", loadAssistResponse.GCPManaged).
			Str("upgrade_subscription_uri", loadAssistResponse.UpgradeSubscriptionURI).
			Msg("Startup authentication check successful.")

		if loadAssistResponse.PaidTier != nil {
			paidTier := loadAssistResponse.PaidTier
			logger.Get().Info().
				Str("paid_tier_id", paidTier.ID).
				Str("paid_tier_name", paidTier.Name).
				Str("paid_tier_description", paidTier.Description).
				Str("upgrade_subscription_uri", paidTier.UpgradeSubscriptionURI).
				Str("upgrade_subscription_text", paidTier.UpgradeSubscriptionText).
				Str("upgrade_subscription_type", paidTier.UpgradeSubscriptionType).
				Interface("available_credits", paidTier.AvailableCredits).
				Msg("Paid tier available.")
		} else {
			logger.Get().Info().Msg("No paid tier returned by loadCodeAssist.")
		}
	}

	// Discover project ID
	envProjectID, _ := env.Get("CLOUDCODE_GCP_PROJECT_ID")
	projectID, err := project.Discover(provider, envProjectID, loadAssistResponse)
	if err != nil {
		logger.Get().Fatal().Err(err).Msg("Failed to discover project ID")
	}

	// Create server with provider and project ID
	srv := server.NewServer(provider, projectID)

	// Start server
	if err := srv.Start(":" + port); err != nil {
		logger.Get().Fatal().Err(err).Msg("Failed to start server")
	}
}
