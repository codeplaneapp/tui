# Node Chat Logs Tab

## Metadata
- ID: runs-task-tab-chat-logs
- Group: Runs And Inspection (runs-and-inspection)
- Type: feature
- Feature: RUNS_TASK_TAB_CHAT_LOGS
- Dependencies: runs-node-inspector

## Summary

Implement the Chat Logs tab to view detailed agent communication or raw execution logs for the node.

## Acceptance Criteria

- Displays the agent interaction or system logs for the node execution
- Supports scrolling through long logs

## Source Context

- internal/ui/views/tasktabs.go

## Implementation Notes

- Use Bubbles viewport for scrolling content.
