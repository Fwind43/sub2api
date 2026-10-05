-- Allow CommandCode / ClinePass as composite route target platforms (and quota platforms).
--
-- 1. user_platform_quotas.platform CHECK
-- 2. composite_model_routes.target_platform CHECK
--
-- CommandCode / ClinePass are OpenAI-compatible relay platforms; composite groups
-- may route to them when the group owns accounts of that platform. The admin API
-- validation (CompositeRouteRequest.TargetPlatform oneof, compositeTargetPlatformAllowed,
-- isConcreteRequestPlatform, matchingPlatforms, compositeAvailableModels) accepts them
-- as of the same change set.
--
-- Runs after 241_add_typesafe_platform.sql. DROP ... IF EXISTS keeps it re-runnable;
-- the new constraint is a superset of 241, so existing rows validate immediately.

ALTER TABLE user_platform_quotas
    DROP CONSTRAINT IF EXISTS user_platform_quotas_platform_check;

ALTER TABLE user_platform_quotas
    ADD CONSTRAINT user_platform_quotas_platform_check
    CHECK (platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok',
                        'kimi', 'zhipu', 'deepseek', 'minimax', 'opencode_go',
                        'commandcode', 'clinepass', 'typesafe'));

ALTER TABLE composite_model_routes
    DROP CONSTRAINT IF EXISTS composite_model_routes_target_platform_check;

ALTER TABLE composite_model_routes
    ADD CONSTRAINT composite_model_routes_target_platform_check
    CHECK (target_platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok',
                               'kimi', 'zhipu', 'deepseek', 'minimax', 'opencode_go',
                               'commandcode', 'clinepass', 'typesafe'));
