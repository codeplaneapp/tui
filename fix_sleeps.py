import re
import sys

files = [
    "internal/e2e/high_impact_e2e_test.go",
    "internal/e2e/high_impact_batch2_e2e_test.go",
    "internal/e2e/high_impact_batch3_e2e_test.go",
    "internal/e2e/high_impact_batch4_e2e_test.go",
    "internal/e2e/runs_inspection_e2e_test.go"
]

def process_file(filepath):
    with open(filepath, 'r') as f:
        lines = f.readlines()

    out_lines = []
    i = 0
    while i < len(lines):
        line = lines[i]
        
        # Check if this line is a time.Sleep
        if re.search(r'\btime\.Sleep\(', line):
            # Look ahead up to 3 lines (ignoring comments and empty lines)
            is_redundant = False
            lookahead_idx = i + 1
            lines_checked = 0
            
            while lookahead_idx < len(lines) and lines_checked < 3:
                next_line = lines[lookahead_idx].strip()
                if not next_line or next_line.startswith('//'):
                    # Empty or comment, just increment index but not lines_checked
                    lookahead_idx += 1
                    # Actually, the instruction says "within 2-3 lines of code/comments"
                    # So we should count any line
                    lines_checked += 1
                    continue
                
                if re.search(r'(WaitForText|WaitForAnyText|WaitForNoText|require\.NoError)', next_line):
                    is_redundant = True
                    break
                
                lookahead_idx += 1
                lines_checked += 1
                
            if is_redundant:
                # Skip this line
                print(f"Removed time.Sleep at {filepath}:{i+1}")
                i += 1
                continue
                
        out_lines.append(line)
        i += 1

    with open(filepath, 'w') as f:
        f.writelines(out_lines)

for f in files:
    process_file(f)

