.PHONY: test test-cover test-cover-html

test:
	go test ./internal/...

test-cover:
	go test ./internal/... -coverprofile=coverage.out
	@go tool cover -func=coverage.out | grep total | awk '{print $$3}'
	@echo "---"
	@go tool cover -func=coverage.out | grep -E "^github" | awk '{split($$1,a,"/"); pkg=a[length(a)-1]"/"a[length(a)]; print pkg, $$NF}' | sort -t'.' -k1 -rn

test-cover-html: test-cover
	go tool cover -html=coverage.out -o coverage.html
	@echo "Open coverage.html in browser"
