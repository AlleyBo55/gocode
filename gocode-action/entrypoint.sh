#!/usr/bin/env bash
set -euo pipefail

# entrypoint.sh — handles PR comment (@gocode mentions) and issue assignment triggers.
# Expects GITHUB_TOKEN, GITHUB_EVENT_PATH, GITHUB_REPOSITORY, and GOCODE_TRIGGER env vars.

API_BASE="https://api.github.com/repos/${GITHUB_REPOSITORY}"

# post_comment creates or updates a comment on an issue/PR.
post_comment() {
  local issue_number="$1"
  local body="$2"
  curl -s -X POST \
    -H "Authorization: token ${GITHUB_TOKEN}" \
    -H "Accept: application/vnd.github+json" \
    "${API_BASE}/issues/${issue_number}/comments" \
    -d "$(jq -n --arg body "$body" '{body: $body}')"
}

# update_comment updates an existing comment by ID.
update_comment() {
  local comment_id="$1"
  local body="$2"
  curl -s -X PATCH \
    -H "Authorization: token ${GITHUB_TOKEN}" \
    -H "Accept: application/vnd.github+json" \
    "${API_BASE}/issues/comments/${comment_id}" \
    -d "$(jq -n --arg body "$body" '{body: $body}')"
}

# progress_comment creates a progress-tracking comment with checkboxes.
progress_comment() {
  local issue_number="$1"
  local title="$2"
  local body="## 🤖 gocode: ${title}

- [ ] Analyzing request
- [ ] Planning changes
- [ ] Implementing changes
- [ ] Verifying results
"
  local result
  result=$(post_comment "$issue_number" "$body")
  echo "$result" | jq -r '.id'
}

# update_progress updates a checkbox in the progress comment.
update_progress() {
  local comment_id="$1"
  local step="$2"
  local issue_number="$3"

  local current_body
  current_body=$(curl -s \
    -H "Authorization: token ${GITHUB_TOKEN}" \
    -H "Accept: application/vnd.github+json" \
    "${API_BASE}/issues/comments/${comment_id}" | jq -r '.body')

  local updated_body
  updated_body=$(echo "$current_body" | sed "s/- \\[ \\] ${step}/- [x] ${step}/")
  update_comment "$comment_id" "$updated_body"
}

# handle_pr_comment parses the event payload for @gocode mentions in PR comments.
handle_pr_comment() {
  local comment_body
  comment_body=$(jq -r '.comment.body // empty' "$GITHUB_EVENT_PATH")

  if [ -z "$comment_body" ]; then
    echo "No comment body found in event payload."
    exit 0
  fi

  # Check for @gocode mention
  if ! echo "$comment_body" | grep -qi "@gocode"; then
    echo "Comment does not mention @gocode. Skipping."
    exit 0
  fi

  local issue_number
  issue_number=$(jq -r '.issue.number // .pull_request.number // empty' "$GITHUB_EVENT_PATH")
  if [ -z "$issue_number" ]; then
    echo "Could not determine issue/PR number."
    exit 1
  fi

  # Strip the @gocode mention to get the prompt
  local prompt
  prompt=$(echo "$comment_body" | sed 's/@gocode//gi' | xargs)

  if [ -z "$prompt" ]; then
    prompt="Review this PR and provide feedback."
  fi

  local progress_id
  progress_id=$(progress_comment "$issue_number" "Processing PR comment")

  update_progress "$progress_id" "Analyzing request" "$issue_number"

  update_progress "$progress_id" "Planning changes" "$issue_number"

  local output
  output=$(gocode prompt --output-format json "$prompt" 2>&1) || true

  update_progress "$progress_id" "Implementing changes" "$issue_number"

  # Post the result as a comment
  local result
  result=$(echo "$output" | jq -r '.result // "No result produced."' 2>/dev/null || echo "$output")
  post_comment "$issue_number" "### gocode result

${result}"

  update_progress "$progress_id" "Verifying results" "$issue_number"
}

# handle_issue_assign handles issue assignment trigger.
handle_issue_assign() {
  local assignee
  assignee=$(jq -r '.assignee.login // empty' "$GITHUB_EVENT_PATH")

  if [ -z "$assignee" ]; then
    echo "No assignee found in event payload."
    exit 0
  fi

  local issue_number
  issue_number=$(jq -r '.issue.number // empty' "$GITHUB_EVENT_PATH")
  local issue_title
  issue_title=$(jq -r '.issue.title // "Untitled"' "$GITHUB_EVENT_PATH")
  local issue_body
  issue_body=$(jq -r '.issue.body // ""' "$GITHUB_EVENT_PATH")

  if [ -z "$issue_number" ]; then
    echo "Could not determine issue number."
    exit 1
  fi

  local progress_id
  progress_id=$(progress_comment "$issue_number" "Implementing: ${issue_title}")

  update_progress "$progress_id" "Analyzing request" "$issue_number"

  local prompt="Implement the following issue:

Title: ${issue_title}

${issue_body}"

  update_progress "$progress_id" "Planning changes" "$issue_number"

  local output
  output=$(gocode prompt --output-format json "$prompt" 2>&1) || true

  update_progress "$progress_id" "Implementing changes" "$issue_number"

  local result
  result=$(echo "$output" | jq -r '.result // "No result produced."' 2>/dev/null || echo "$output")
  post_comment "$issue_number" "### gocode result

${result}"

  update_progress "$progress_id" "Verifying results" "$issue_number"
}

# Main dispatch
case "${GOCODE_TRIGGER:-manual}" in
  pr-comment)
    handle_pr_comment
    ;;
  issue-assign)
    handle_issue_assign
    ;;
  *)
    echo "Unknown trigger: ${GOCODE_TRIGGER}. Use 'pr-comment' or 'issue-assign'."
    exit 1
    ;;
esac
