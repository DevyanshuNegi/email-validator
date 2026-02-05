# SMTP Response Code Categorization Guide

## Exact Categorization Rules

### 550 - User Unknown

**Category:** `permanent_failure` (Hard Bounce)  
**Type:** `user_unknown`  
**Action:** `reject`  
**Validation Result:** `invalid`  
**Retry:** NO - Do NOT retry  
**Database Status:** `invalid`

**Explanation:** This is a permanent failure indicating the recipient mailbox does not exist. The user is unknown to the mail server. This is a HARD BOUNCE - mark as invalid immediately and do not attempt to retry.

---

### 452 - Inbox Full

**Category:** `temporary_failure` (Soft Bounce)  
**Type:** `inbox_full`  
**Action:** `retry`  
**Validation Result:** `temporary_failure`  
**Retry Strategy:**
- Initial delay: 3600 seconds (1 hour)
- Max retries: 2
- Backoff multiplier: 1.5
- Database Status: `pending_retry`

**Explanation:** This is a temporary failure indicating insufficient system storage. The inbox is full or the server is overloaded. This is a SOFT BOUNCE - retry after a longer delay (1 hour), with a maximum of 2 retry attempts.

---

### 451 - Greylisting

**Category:** `temporary_failure` (Soft Bounce)  
**Type:** `greylisting`  
**Action:** `retry`  
**Validation Result:** `temporary_failure`  
**Retry Strategy:**
- Initial delay: 300 seconds (5 minutes)
- Max retries: 3
- Backoff multiplier: 2 (exponential backoff)
- Database Status: `pending_retry`

**Explanation:** This is a temporary failure often caused by greylisting (a spam prevention technique where servers temporarily reject mail from unknown senders). The server is requesting a retry. This is a SOFT BOUNCE - MUST retry with exponential backoff, up to 3 attempts.

**Note:** According to your architecture rules, you must implement retry logic for SMTP 450/451 (Greylisting).

---

### Catch-All Domain Detection

**There is NO single RFC code that definitively indicates a catch-all domain.** Catch-all detection requires **behavioral analysis**.

#### Strong Indicators (Single Code):

**252 - Cannot VRFY but will accept**
- **Category:** `success`
- **Type:** `catchall_strong_indicator`
- **Validation Result:** `catchall_detected`
- **Confidence:** HIGH
- **Meaning:** Server cannot verify the user but will accept the message anyway. This is the strongest single-code indicator of catch-all behavior.

#### Medium Indicators:

**251 - User not local; will forward**
- **Category:** `success`
- **Type:** `valid_forward`
- **Catchall Indicator:** `medium`
- **Confidence:** MEDIUM
- **Meaning:** Indicates forwarding behavior, which may suggest catch-all domain.

#### Detection Algorithm:

1. **Step 1:** Perform normal validation with the real email address
2. **Step 2:** If response is 250, 251, or 252, proceed to catch-all test
3. **Step 3:** Test with a random invalid address (e.g., `test12345@domain.com`)
4. **Step 4:** Compare responses:
   - If invalid address returns **250/251/252** → Domain is **CATCH-ALL**
   - If invalid address returns **550** → Domain is **NOT catch-all** (normal validation)

#### Code 250 Behavior:

- **250 OK** alone does not indicate catch-all
- **250 OK** for both valid AND invalid addresses → Catch-all detected
- Must test with invalid address to confirm

---

## Decision Tree Summary

```
SMTP Response Code
│
├─ 2xx (Success)
│  ├─ 250 → Valid (test for catch-all)
│  ├─ 251 → Valid + Medium catch-all indicator
│  └─ 252 → Valid + Strong catch-all indicator (likely catch-all)
│
├─ 4xx (Temporary Failure)
│  ├─ 450 → Greylisting (retry after 60s, max 3)
│  ├─ 451 → Greylisting (retry after 300s, max 3, exponential backoff)
│  └─ 452 → Inbox Full (retry after 3600s, max 2)
│
└─ 5xx (Permanent Failure)
   └─ 550 → User Unknown (reject, do not retry)
```

---

## Implementation Notes

1. **550 User Unknown:** Hard bounce - mark as invalid immediately, no retries
2. **452 Inbox Full:** Soft bounce - retry after 1 hour, max 2 attempts
3. **451 Greylisting:** Soft bounce - retry after 5 minutes with exponential backoff, max 3 attempts
4. **Catch-All Detection:** Requires testing with invalid address after receiving 250/251/252
5. **Code 252:** Strongest single-code indicator of catch-all (but still verify with invalid address test)

---

## Files Created

- `smtp_codes.json` - Complete JSON mapping of all SMTP codes
- `smtp_types.go` - Go types and helper functions for code handling
- `SMTP_CATEGORIZATION.md` - This documentation file
