GOLANGCI_LINT := $(GOPATH)/bin/golangci-lint
GOBIN = $(GOPATH)/bin
GOTEST = $(GOBIN)/gotest

lint: 
	@echo Linting...
	@$(GOLANGCI_LINT)  -v --concurrency=3 --config=.golangci.yml --issues-exit-code=0 run \
	--out-format=colored-line-number

test:  $(GOTEST)
	@mkdir -p reports
	LOGFORMAT=ASCII gotest -covermode=count -p=1 -v -coverprofile reports/codecoverage_all.cov `go list ./...`
	@echo "Done running tests"
	@go tool cover -func=reports/codecoverage_all.cov > reports/functioncoverage.out
	@go tool cover -html=reports/codecoverage_all.cov -o reports/coverage.html
	@echo "View report at $(PWD)/reports/coverage.html"
	@tail -n 1 reports/functioncoverage.out

coverage-report:
	@open reports/coverage.html