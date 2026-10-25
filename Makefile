.PHONY: up down logs debug debug-check test e2e integration seed lint deploy
up:
	docker compose up --build -d

down:
	docker compose down

logs:
	docker compose logs -f api worker seed

debug:
	docker compose -f compose.yml -f compose.debug.yml up --build -d

seed:
	docker compose run --rm seed

test:
	cd app && go test -race ./...

e2e:
	node tests/e2e.mjs

lint:
	cd app && go vet ./...
	cfn-lint infrastructure/template.yml infrastructure/network.yml
	python3 -m unittest discover -s tests -p '*_test.py'

deploy:
	./scripts/aws/deploy.sh $(ENVIRONMENT) $(CONFIG)

# Requires the local Compose emulators; uses only synthetic source records.
integration:
	cd app && SETU_INTEGRATION=1 go test ./internal/platform -run 'Test(Migration|AssessmentResults)AgainstLocalEmulators' -v

debug-check:
	node tests/debug.mjs
