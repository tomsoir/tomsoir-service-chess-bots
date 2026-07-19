# ---------------------------------------------------------------------------
# Deploy: commit, push, bump patch tag, push tag (triggers GitHub Actions)
# Usage: make deploy "commit message"
#    or: make deploy MSG="commit message"
# ---------------------------------------------------------------------------
ifeq (deploy,$(firstword $(MAKECMDGOALS)))
  DEPLOY_ARGS := $(wordlist 2,$(words $(MAKECMDGOALS)),$(MAKECMDGOALS))
  %:
	@:
endif

.PHONY: deploy
deploy:
	@set -e; \
	MSG="$(if $(MSG),$(MSG),$(DEPLOY_ARGS))"; \
	if [ -z "$$MSG" ]; then \
		echo 'Usage: make deploy "commit message"'; \
		exit 1; \
	fi; \
	git fetch --tags --quiet; \
	LAST_TAG=$$(git tag --sort=-v:refname | head -n 1); \
	if [ -z "$$LAST_TAG" ]; then LAST_TAG="0.0.0"; fi; \
	NEW_TAG=$$(echo "$$LAST_TAG" | awk -F. '{printf "%d.%d.%d", $$1, $$2, $$3 + 1}'); \
	if git diff --quiet && git diff --cached --quiet && [ -z "$$(git ls-files --others --exclude-standard)" ]; then \
		echo "→ Working tree clean, skipping commit"; \
	else \
		echo "→ Commit: $$MSG"; \
		git add -A; \
		git commit -m "$$MSG"; \
	fi; \
	BRANCH=$$(git rev-parse --abbrev-ref HEAD); \
	echo "→ Push $$BRANCH"; \
	git push origin "$$BRANCH"; \
	echo "→ Tag $$LAST_TAG → $$NEW_TAG"; \
	git tag "$$NEW_TAG"; \
	echo "→ Push tag $$NEW_TAG"; \
	git push origin "$$NEW_TAG"; \
	echo "✓ Deploy triggered ($$NEW_TAG)"
