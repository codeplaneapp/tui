# Run DAG Overview

## Metadata
- ID: runs-dag-overview
- Group: Runs And Inspection (runs-and-inspection)
- Type: feature
- Feature: RUNS_DAG_OVERVIEW
- Dependencies: runs-inspect-summary

## Summary

Render an ASCII/Unicode visualization of the node execution graph in the Run Inspector.

## Acceptance Criteria

- Shows nodes and their dependencies as a tree or graph
- Highlights active and failed nodes

## Source Context

- internal/ui/components/dagview.go
- internal/ui/views/runinspect.go

## Implementation Notes

- A simple vertical tree representation (like git log --graph) may suffice for TUI.
