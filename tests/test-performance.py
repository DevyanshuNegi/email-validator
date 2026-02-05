#!/usr/bin/env python3
"""
Email Performance Test Script
Tests emails individually, tracks comprehensive metrics, and generates detailed reports.
Similar to emailtester.ninja functionality.
"""

import csv
import json
import time
import sys
from datetime import datetime
from typing import Dict, List, Optional, Tuple
import requests
from collections import defaultdict

# Try to import tqdm for progress bar, fallback gracefully
try:
    from tqdm import tqdm
    HAS_TQDM = True
except ImportError:
    HAS_TQDM = False
    print("Note: tqdm not available. Install with 'pip install tqdm' for progress bar.")

# Configuration
API_BASE_URL = "http://localhost:8080"
API_VERIFY_ENDPOINT = f"{API_BASE_URL}/api/verify"
API_JOB_ENDPOINT = f"{API_BASE_URL}/api/job"
POLL_INTERVAL = 0.5  # 500ms
MAX_WAIT_TIME = 60  # seconds per email
INITIAL_POLL_INTERVAL = 0.5
EXPONENTIAL_BACKOFF_START = 5  # seconds before exponential backoff
MAX_RETRIES = 3
RETRY_DELAY = 1  # seconds

# Status mapping for expected vs actual comparison
EXPECTED_TO_ACTUAL = {
    'valid': ['VALID', 'CATCH_ALL'],
    'invalid': ['INVALID'],
    'disposable': ['VALID', 'CATCH_ALL', 'INVALID']  # Disposable can be any
}


class TestResult:
    """Stores test result for a single email"""
    def __init__(self, email: str, expected_result: str, category: str):
        self.email = email
        self.expected_result = expected_result
        self.category = category
        
        # Timing metrics
        self.timestamp_submitted: Optional[float] = None
        self.timestamp_completed: Optional[float] = None
        self.api_request_time: Optional[float] = None  # ms
        self.queue_time: Optional[float] = None  # ms
        self.processing_time: Optional[float] = None  # ms
        self.total_time: Optional[float] = None  # ms
        self.poll_count = 0
        self.poll_intervals: List[float] = []
        
        # Result metrics
        self.job_id: Optional[str] = None
        self.actual_status: Optional[str] = None
        self.smtp_code: Optional[int] = None
        self.bounce_reason: Optional[str] = None
        self.match_expected: Optional[bool] = None
        
        # Error tracking
        self.error: Optional[str] = None
        self.timeout = False

    def to_dict(self) -> Dict:
        """Convert to dictionary for JSON/CSV output"""
        return {
            'email': self.email,
            'category': self.category,
            'expected_result': self.expected_result,
            'actual_status': self.actual_status,
            'smtp_code': self.smtp_code,
            'bounce_reason': self.bounce_reason,
            'match_expected': self.match_expected,
            'api_request_time': round(self.api_request_time, 2) if self.api_request_time else None,
            'queue_time': round(self.queue_time, 2) if self.queue_time else None,
            'processing_time': round(self.processing_time, 2) if self.processing_time else None,
            'total_time': round(self.total_time, 2) if self.total_time else None,
            'poll_count': self.poll_count,
            'poll_interval_avg': round(sum(self.poll_intervals) / len(self.poll_intervals), 2) if self.poll_intervals else None,
            'timestamp_submitted': datetime.fromtimestamp(self.timestamp_submitted).isoformat() if self.timestamp_submitted else None,
            'timestamp_completed': datetime.fromtimestamp(self.timestamp_completed).isoformat() if self.timestamp_completed else None,
            'job_id': self.job_id,
            'error': self.error,
            'timeout': self.timeout
        }

    def to_csv_row(self) -> Dict:
        """Convert to CSV row format"""
        d = self.to_dict()
        # Flatten poll_interval_avg
        d['poll_interval_avg'] = d.pop('poll_interval_avg')
        return d


def read_test_emails(csv_file: str) -> List[Tuple[str, str, str]]:
    """Read emails from CSV file"""
    emails = []
    try:
        with open(csv_file, 'r', encoding='utf-8') as f:
            reader = csv.DictReader(f)
            for row in reader:
                email = row['email'].strip()
                expected = row['expected_result'].strip()
                category = row['category'].strip()
                emails.append((email, expected, category))
    except FileNotFoundError:
        print(f"Error: {csv_file} not found!")
        sys.exit(1)
    except Exception as e:
        print(f"Error reading {csv_file}: {e}")
        sys.exit(1)
    
    return emails


def submit_email(email: str) -> Tuple[Optional[str], Optional[str], float]:
    """
    Submit email to API for validation
    Returns: (job_id, error_message, request_time_ms)
    """
    start_time = time.time()
    try:
        response = requests.post(
            API_VERIFY_ENDPOINT,
            json=[email],
            headers={'Content-Type': 'application/json'},
            timeout=10
        )
        request_time = (time.time() - start_time) * 1000  # Convert to ms
        
        if response.status_code == 201:
            data = response.json()
            return data.get('jobId'), None, request_time
        else:
            error_msg = f"API returned {response.status_code}: {response.text}"
            return None, error_msg, request_time
    except requests.exceptions.Timeout:
        request_time = (time.time() - start_time) * 1000
        return None, "API request timeout", request_time
    except requests.exceptions.ConnectionError:
        request_time = (time.time() - start_time) * 1000
        return None, "Connection error - is the API running?", request_time
    except Exception as e:
        request_time = (time.time() - start_time) * 1000
        return None, f"Unexpected error: {str(e)}", request_time


def poll_job_status(job_id: str, result: TestResult) -> bool:
    """
    Poll job status until completion or timeout
    Returns: True if completed, False if timeout/error
    """
    start_poll_time = time.time()
    last_poll_time = start_poll_time
    poll_interval = INITIAL_POLL_INTERVAL
    
    while True:
        elapsed = time.time() - start_poll_time
        
        # Check timeout
        if elapsed > MAX_WAIT_TIME:
            result.timeout = True
            result.error = f"Timeout after {MAX_WAIT_TIME}s"
            return False
        
        # Calculate current poll interval (exponential backoff after threshold)
        if elapsed > EXPONENTIAL_BACKOFF_START:
            poll_interval = min(poll_interval * 1.2, 2.0)  # Max 2s interval
        
        # Wait before polling (except first time)
        if result.poll_count > 0:
            time.sleep(poll_interval)
        
        # Poll API
        try:
            response = requests.get(
                f"{API_JOB_ENDPOINT}/{job_id}",
                timeout=5
            )
            
            if response.status_code != 200:
                result.error = f"Job status API returned {response.status_code}"
                return False
            
            data = response.json()
            result.poll_count += 1
            
            # Calculate poll interval
            current_time = time.time()
            if result.poll_count > 1:
                interval = (current_time - last_poll_time) * 1000  # ms
                result.poll_intervals.append(interval)
            last_poll_time = current_time
            
            # Check if job is complete
            email_checks = data.get('emailChecks', [])
            if email_checks:
                check = email_checks[0]  # Single email per job
                status = check.get('status')
                
                if status != 'PENDING':
                    # Job completed
                    result.actual_status = status
                    result.smtp_code = check.get('smtpCode')
                    result.bounce_reason = check.get('bounceReason')
                    result.timestamp_completed = time.time()
                    return True
                    
        except requests.exceptions.Timeout:
            result.error = "Polling timeout"
            return False
        except requests.exceptions.ConnectionError:
            result.error = "Connection error during polling"
            return False
        except Exception as e:
            result.error = f"Polling error: {str(e)}"
            return False


def compare_results(result: TestResult) -> bool:
    """Compare expected vs actual result"""
    if result.actual_status is None:
        return False
    
    expected = result.expected_result.lower()
    actual = result.actual_status.upper()
    
    # Handle special cases
    if actual == 'UNKNOWN':
        # Port 25 blocked or connection issue - mark as mismatch but note reason
        result.match_expected = False
        return False
    
    if actual == 'GREYLISTED':
        # Temporary failure - could be valid, mark as uncertain
        result.match_expected = expected == 'valid'
        return result.match_expected
    
    # Standard mapping
    valid_statuses = EXPECTED_TO_ACTUAL.get(expected, [])
    result.match_expected = actual in valid_statuses
    return result.match_expected


def test_single_email(email: str, expected_result: str, category: str) -> TestResult:
    """Test a single email and return result"""
    result = TestResult(email, expected_result, category)
    result.timestamp_submitted = time.time()
    
    # Submit email
    job_id, error, api_time = submit_email(email)
    result.api_request_time = api_time
    
    if error:
        result.error = error
        return result
    
    result.job_id = job_id
    
    # Poll for completion
    completed = poll_job_status(job_id, result)
    
    if completed:
        # Calculate timing metrics
        if result.timestamp_completed and result.timestamp_submitted:
            total_time = (result.timestamp_completed - result.timestamp_submitted) * 1000  # ms
            result.total_time = total_time
            
            # Estimate queue time (time from submission to first non-pending status)
            # For single email jobs, queue time is roughly: total_time - processing_time
            # Processing time is harder to measure precisely, so we estimate:
            # queue_time = time until first poll that shows completion
            if result.poll_count > 0 and result.poll_intervals:
                # Queue time ≈ time until processing started
                # Estimate: first poll interval or average of early polls
                result.queue_time = sum(result.poll_intervals[:min(3, len(result.poll_intervals))]) / min(3, len(result.poll_intervals))
                result.processing_time = result.total_time - result.api_request_time - (result.queue_time if result.queue_time else 0)
            else:
                result.processing_time = result.total_time - result.api_request_time
        
        # Compare results
        compare_results(result)
    
    return result


def calculate_percentiles(values: List[float], percentiles: List[int]) -> Dict[int, float]:
    """Calculate percentiles from a list of values"""
    if not values:
        return {p: 0.0 for p in percentiles}
    
    sorted_values = sorted(values)
    result = {}
    for p in percentiles:
        index = int(len(sorted_values) * p / 100)
        index = min(index, len(sorted_values) - 1)
        result[p] = sorted_values[index]
    return result


def generate_summary(results: List[TestResult]) -> Dict:
    """Generate summary statistics"""
    total = len(results)
    completed = [r for r in results if r.actual_status is not None]
    matched = [r for r in completed if r.match_expected]
    errors = [r for r in results if r.error]
    timeouts = [r for r in results if r.timeout]
    
    # Timing statistics
    total_times = [r.total_time for r in completed if r.total_time]
    api_times = [r.api_request_time for r in results if r.api_request_time]
    processing_times = [r.processing_time for r in completed if r.processing_time and r.processing_time > 0]
    
    # Category breakdown
    category_stats = defaultdict(lambda: {'total': 0, 'completed': 0, 'matched': 0, 'errors': 0})
    for r in results:
        cat = r.category
        category_stats[cat]['total'] += 1
        if r.actual_status:
            category_stats[cat]['completed'] += 1
        if r.match_expected:
            category_stats[cat]['matched'] += 1
        if r.error:
            category_stats[cat]['errors'] += 1
    
    # Status breakdown
    status_counts = defaultdict(int)
    for r in completed:
        status_counts[r.actual_status] += 1
    
    summary = {
        'total_emails': total,
        'completed': len(completed),
        'errors': len(errors),
        'timeouts': len(timeouts),
        'success_rate': (len(matched) / len(completed) * 100) if completed else 0,
        'completion_rate': (len(completed) / total * 100) if total > 0 else 0,
        'timing': {
            'total_time': {
                'min': min(total_times) if total_times else 0,
                'max': max(total_times) if total_times else 0,
                'avg': sum(total_times) / len(total_times) if total_times else 0,
                'percentiles': calculate_percentiles(total_times, [50, 90, 95, 99])
            },
            'api_time': {
                'min': min(api_times) if api_times else 0,
                'max': max(api_times) if api_times else 0,
                'avg': sum(api_times) / len(api_times) if api_times else 0
            },
            'processing_time': {
                'min': min(processing_times) if processing_times else 0,
                'max': max(processing_times) if processing_times else 0,
                'avg': sum(processing_times) / len(processing_times) if processing_times else 0
            }
        },
        'category_breakdown': dict(category_stats),
        'status_breakdown': dict(status_counts),
        'throughput': len(completed) / (sum(total_times) / 1000) if total_times else 0  # emails per second
    }
    
    return summary


def write_csv_output(results: List[TestResult], filename: str):
    """Write results to CSV file"""
    if not results:
        return
    
    fieldnames = [
        'email', 'category', 'expected_result', 'actual_status', 'smtp_code',
        'bounce_reason', 'match_expected', 'api_request_time', 'queue_time',
        'processing_time', 'total_time', 'poll_count', 'poll_interval_avg',
        'timestamp_submitted', 'timestamp_completed', 'job_id', 'error', 'timeout'
    ]
    
    with open(filename, 'w', newline='', encoding='utf-8') as f:
        writer = csv.DictWriter(f, fieldnames=fieldnames)
        writer.writeheader()
        for result in results:
            writer.writerow(result.to_csv_row())


def write_json_output(results: List[TestResult], summary: Dict, filename: str):
    """Write results to JSON file"""
    output = {
        'test_metadata': {
            'timestamp': datetime.now().isoformat(),
            'total_emails': len(results),
            'api_endpoint': API_VERIFY_ENDPOINT
        },
        'summary': summary,
        'results': [r.to_dict() for r in results]
    }
    
    with open(filename, 'w', encoding='utf-8') as f:
        json.dump(output, f, indent=2, ensure_ascii=False)


def write_summary_report(results: List[TestResult], summary: Dict, filename: str):
    """Write human-readable summary report"""
    with open(filename, 'w', encoding='utf-8') as f:
        f.write("=" * 80 + "\n")
        f.write("EMAIL VALIDATION PERFORMANCE TEST REPORT\n")
        f.write("=" * 80 + "\n\n")
        f.write(f"Test Date: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}\n")
        f.write(f"API Endpoint: {API_VERIFY_ENDPOINT}\n\n")
        
        f.write("OVERALL STATISTICS\n")
        f.write("-" * 80 + "\n")
        f.write(f"Total Emails Tested: {summary['total_emails']}\n")
        f.write(f"Completed: {summary['completed']} ({summary['completion_rate']:.1f}%)\n")
        f.write(f"Errors: {summary['errors']}\n")
        f.write(f"Timeouts: {summary['timeouts']}\n")
        f.write(f"Success Rate (Matches Expected): {summary['success_rate']:.1f}%\n")
        f.write(f"Throughput: {summary['throughput']:.2f} emails/second\n\n")
        
        f.write("TIMING STATISTICS\n")
        f.write("-" * 80 + "\n")
        timing = summary['timing']
        f.write(f"Total Time (ms):\n")
        f.write(f"  Min: {timing['total_time']['min']:.2f}\n")
        f.write(f"  Max: {timing['total_time']['max']:.2f}\n")
        f.write(f"  Avg: {timing['total_time']['avg']:.2f}\n")
        f.write(f"  P50: {timing['total_time']['percentiles'].get(50, 0):.2f}\n")
        f.write(f"  P90: {timing['total_time']['percentiles'].get(90, 0):.2f}\n")
        f.write(f"  P95: {timing['total_time']['percentiles'].get(95, 0):.2f}\n")
        f.write(f"  P99: {timing['total_time']['percentiles'].get(99, 0):.2f}\n\n")
        
        f.write(f"API Request Time (ms):\n")
        f.write(f"  Min: {timing['api_time']['min']:.2f}\n")
        f.write(f"  Max: {timing['api_time']['max']:.2f}\n")
        f.write(f"  Avg: {timing['api_time']['avg']:.2f}\n\n")
        
        f.write(f"Processing Time (ms):\n")
        f.write(f"  Min: {timing['processing_time']['min']:.2f}\n")
        f.write(f"  Max: {timing['processing_time']['max']:.2f}\n")
        f.write(f"  Avg: {timing['processing_time']['avg']:.2f}\n\n")
        
        f.write("STATUS BREAKDOWN\n")
        f.write("-" * 80 + "\n")
        for status, count in summary['status_breakdown'].items():
            f.write(f"  {status}: {count}\n")
        f.write("\n")
        
        f.write("CATEGORY BREAKDOWN\n")
        f.write("-" * 80 + "\n")
        for category, stats in summary['category_breakdown'].items():
            match_rate = (stats['matched'] / stats['completed'] * 100) if stats['completed'] > 0 else 0
            f.write(f"  {category}:\n")
            f.write(f"    Total: {stats['total']}, Completed: {stats['completed']}, Matched: {stats['matched']} ({match_rate:.1f}%), Errors: {stats['errors']}\n")
        f.write("\n")
        
        f.write("ERRORS\n")
        f.write("-" * 80 + "\n")
        error_results = [r for r in results if r.error]
        if error_results:
            for r in error_results:
                f.write(f"  {r.email}: {r.error}\n")
        else:
            f.write("  No errors\n")


def main():
    """Main test execution"""
    print("Email Performance Test Script")
    print("=" * 80)
    
    # Read test emails (use small CSV if provided as argument, otherwise default)
    default_csv = 'data/test-emails.csv'
    csv_file = sys.argv[1] if len(sys.argv) > 1 else default_csv
    print(f"Reading test emails from {csv_file}...")
    all_test_emails = read_test_emails(csv_file)
    print(f"Found {len(all_test_emails)} total emails in file\n")
    
    timestamp = datetime.now().strftime('%Y%m%d_%H%M%S')
    error_log_file = f"../logs/test-errors-{timestamp}.log"
    
    # Step 1: Test 5 emails first
    print("=" * 80)
    print("PHASE 1: Testing 5 emails first...")
    print("=" * 80)
    
    test_emails_5 = all_test_emails[:5]
    results_5 = []
    
    # Progress tracking for 5 emails
    if HAS_TQDM:
        iterator = tqdm(test_emails_5, desc="Testing 5 emails", unit="email")
    else:
        iterator = test_emails_5
        print("Testing 5 emails...")
    
    for i, (email, expected, category) in enumerate(iterator, 1):
        if not HAS_TQDM:
            print(f"[{i}/5] Testing: {email}")
        
        result = test_single_email(email, expected, category)
        results_5.append(result)
        
        # Log errors
        if result.error:
            with open(error_log_file, 'a', encoding='utf-8') as f:
                f.write(f"{datetime.now().isoformat()} - {email}: {result.error}\n")
    
    # Check if 5-email test was successful
    summary_5 = generate_summary(results_5)
    success_threshold = 80.0  # At least 80% completion rate
    
    print("\n" + "=" * 80)
    print("PHASE 1 RESULTS")
    print("=" * 80)
    print(f"Completed: {summary_5['completed']}/{summary_5['total_emails']} ({summary_5['completion_rate']:.1f}%)")
    print(f"Errors: {summary_5['errors']}")
    print(f"Timeouts: {summary_5['timeouts']}")
    
    if summary_5['completion_rate'] < success_threshold:
        print(f"\n❌ Phase 1 failed: Completion rate ({summary_5['completion_rate']:.1f}%) below threshold ({success_threshold}%)")
        print("Stopping tests. Please check your setup before running full test.")
        
        # Still save the 5-email results
        summary_file_5 = f"results/test-summary-5emails-{timestamp}.txt"
        write_summary_report(results_5, summary_5, summary_file_5)
        print(f"✓ Partial results saved to: {summary_file_5}")
        return
    
    print(f"\n✅ Phase 1 successful! Proceeding to test all {len(all_test_emails)} emails...\n")
    
    # Step 2: Test all emails
    print("=" * 80)
    print(f"PHASE 2: Testing all {len(all_test_emails)} emails...")
    print("=" * 80)
    
    # Start with the 5 we already tested
    all_results = results_5.copy()
    
    # Test remaining emails
    remaining_emails = all_test_emails[5:]
    
    if HAS_TQDM:
        iterator = tqdm(remaining_emails, desc="Testing remaining emails", unit="email", initial=5, total=len(all_test_emails))
    else:
        iterator = remaining_emails
        print(f"Testing remaining {len(remaining_emails)} emails...")
    
    for i, (email, expected, category) in enumerate(iterator, 1):
        if not HAS_TQDM:
            print(f"[{5+i}/{len(all_test_emails)}] Testing: {email}")
        
        result = test_single_email(email, expected, category)
        all_results.append(result)
        
        # Log errors
        if result.error:
            with open(error_log_file, 'a', encoding='utf-8') as f:
                f.write(f"{datetime.now().isoformat()} - {email}: {result.error}\n")
    
    print("\n" + "=" * 80)
    print("Test completed! Generating reports...\n")
    
    # Generate summary for all results
    summary = generate_summary(all_results)
    
    # Write output files
    csv_file = f"results/test-results-{timestamp}.csv"
    json_file = f"results/test-results-{timestamp}.json"
    summary_file = f"results/test-summary-{timestamp}.txt"
    
    write_csv_output(all_results, csv_file)
    print(f"✓ CSV output: {csv_file}")
    
    write_json_output(all_results, summary, json_file)
    print(f"✓ JSON output: {json_file}")
    
    write_summary_report(all_results, summary, summary_file)
    print(f"✓ Summary report: {summary_file}")
    
    if any(r.error for r in all_results):
        print(f"✓ Error log: {error_log_file}")
    
    # Print summary to console
    print("\n" + "=" * 80)
    print("FINAL SUMMARY")
    print("=" * 80)
    print(f"Total Emails: {summary['total_emails']}")
    print(f"Completed: {summary['completed']} ({summary['completion_rate']:.1f}%)")
    print(f"Success Rate: {summary['success_rate']:.1f}%")
    print(f"Throughput: {summary['throughput']:.2f} emails/second")
    print(f"Avg Total Time: {summary['timing']['total_time']['avg']:.2f} ms")
    print(f"Avg Processing Time: {summary['timing']['processing_time']['avg']:.2f} ms")
    print("=" * 80)


if __name__ == '__main__':
    main()
