# Auto-Generate Unit Tests Workflow

This GitHub Actions workflow automatically generates unit tests for new or modified Go code in pull requests using AI (GitHub Copilot API with OpenAI fallback).

## Features

- 🤖 **AI-Powered Test Generation**: Uses GitHub Copilot or OpenAI to generate comprehensive unit tests
- 🎯 **Targeted Coverage**: Runs tests only on affected packages, not the entire repo
- 📊 **Coverage Reporting**: Shows coverage metrics for changed packages
- ✅ **Manual Approval**: Requires explicit approval before committing generated tests
- 🔄 **Automatic Updates**: Triggers on every PR update to regenerate tests for new changes

## How It Works

1. **Trigger**: Runs when a PR is opened, updated, or when `/approve-tests` comment is posted
2. **Detection**: Identifies changed `.go` files (excluding test files and EXCLUDE_PACKAGES)
3. **Extraction**: Parses Go AST to find exported functions in changed files
4. **Generation**: Uses AI to generate unit tests following Go best practices
5. **Testing**: Runs `go test` only on affected packages with coverage analysis
6. **Review**: Posts PR comment with generated tests and coverage report
7. **Approval**: Waits for manual approval via `/approve-tests` comment
8. **Commit**: Commits approved tests to the PR branch

## Setup

### Required Permissions

The workflow requires these GitHub token permissions (already configured):
- `contents: write` - To commit generated tests
- `pull-requests: write` - To comment on PRs
- `issues: write` - To handle comment triggers

### API Configuration

#### Option 1: GitHub Copilot API (Recommended)

Uses the built-in `GITHUB_TOKEN` - no additional setup required! The workflow will automatically use GitHub's Models API endpoint.

**Advantages:**
- ✅ No external API key needed
- ✅ Integrated with GitHub
- ✅ Included with GitHub Copilot subscription

#### Option 2: OpenAI API (Fallback)

If GitHub Copilot API is unavailable, add an OpenAI API key:

1. Get an API key from [OpenAI Platform](https://platform.openai.com/api-keys)
2. Add it as a repository secret:
   - Go to: `Settings` → `Secrets and variables` → `Actions`
   - Click `New repository secret`
   - Name: `OPENAI_API_KEY`
   - Value: Your OpenAI API key

The workflow will automatically fall back to OpenAI if GitHub Copilot API fails.

## Usage

### For Contributors

1. **Create a PR** with your Go code changes
2. **Wait for workflow** to analyze changes and generate tests
3. **Review the bot comment** showing generated tests and coverage
4. **Approve tests** by commenting `/approve-tests` if satisfied
5. **Tests are committed** automatically to your PR branch

### Example Workflow

```yaml
# PR opened with changes to core/orchestrator_core.go

→ Workflow detects changes
→ Extracts new/modified functions
→ Generates tests using AI
→ Runs coverage on core package only
→ Posts comment with results

# You review and approve
/approve-tests

→ Tests committed to PR branch
→ Workflow acknowledges approval
```

## Excluded Packages

The following packages are excluded from test generation (matching Makefile EXCLUDE_PACKAGES):

- `client/clientset`
- `client/informers`
- `client/listers`
- `storage/external_test`
- `astrads/api/v1alpha1`
- `ontap/api/azgo`
- `ontap/api/rest`
- `fake` (any fake packages)
- `mocks/` (any mock packages)
- `operator/controllers/provisioner`
- `storage_drivers/astrads/api/v1beta1`

## Generated Test Quality

The AI generates tests following these guidelines:

✅ **Table-driven test patterns** where appropriate  
✅ **Edge cases and error scenarios**  
✅ **Go testing best practices**  
✅ **Proper test function naming** (`TestFunctionName`)  
✅ **Mocking for external dependencies**  
✅ **Standard Go testing package** (compatible with existing tests)

## Coverage Reporting

The workflow provides:

- **Per-package coverage** for affected packages
- **Overall coverage** for changed code
- **Non-blocking** - won't fail the PR, just reports metrics

Example output:
```
## Coverage Report

| Package | Coverage |
|---------|----------|
| core    | 75.3%    |
| storage | 82.1%    |

**Overall coverage for changed packages:** 78.7%
```

## Commands

| Command | Action |
|---------|--------|
| `/approve-tests` | Approve and commit generated tests to PR branch |

## Customization

### Adjust AI Model

Edit the workflow file and change the model parameter:

```python
# For GitHub Copilot API
def call_github_copilot(prompt, model="gpt-4o"):  # Change model here

# For OpenAI API
def call_openai(prompt, model="gpt-4o-mini"):  # Change model here
```

### Modify Test Patterns

Update the prompt in the `generate_tests.py` script:

```python
prompt = f"""Generate a comprehensive unit test...
Requirements:
1. Your custom requirement here
2. Another custom requirement
...
```

### Change Coverage Threshold

Add coverage validation in the "Run tests on affected packages" step:

```bash
COVERAGE_THRESHOLD=70
if (( $(echo "$OVERALL < $COVERAGE_THRESHOLD" | bc -l) )); then
  echo "Warning: Coverage below ${COVERAGE_THRESHOLD}%"
fi
```

## Troubleshooting

### Tests not generated

**Cause**: No exported functions found or all files excluded  
**Solution**: Check that your changes include exported (capitalized) functions

### API errors

**Cause**: Rate limits or API unavailable  
**Solution**: Workflow will try OpenAI fallback if configured

### Coverage not calculated

**Cause**: Test compilation errors or affected packages not found  
**Solution**: Check workflow logs; tests may need manual fixes

### Commit fails

**Cause**: Insufficient permissions  
**Solution**: Ensure workflow has `contents: write` permission

## Workflow Triggers

```yaml
on:
  pull_request:
    types: [opened, synchronize, reopened]
    paths:
      - '**.go'
      - '!**_test.go'
  issue_comment:
    types: [created]
```

## Performance

- **Diff analysis**: ~5-10 seconds
- **Test generation**: ~2-5 seconds per function
- **Coverage calculation**: ~10-30 seconds (only affected packages)
- **Total**: Typically < 2 minutes for most PRs

## Best Practices

1. **Review generated tests** - AI is good but not perfect
2. **Add edge cases** - Supplement AI tests with domain-specific cases
3. **Update existing tests** - Don't rely solely on generated tests
4. **Commit incrementally** - Approve tests after each meaningful change

## Limitations

- Only generates tests for **exported functions** (public API)
- May need manual adjustments for complex mocking scenarios
- Coverage calculated only for **affected packages** (not entire repo)
- Requires manual approval - no automatic commits

## Support

For issues or questions:
1. Check workflow logs in Actions tab
2. Review generated tests in PR comments
3. Open an issue with workflow run link

## License

This workflow is part of the NetApp Trident project.
