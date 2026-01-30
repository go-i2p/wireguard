# Architecture Decision Record Template

## Status

[Proposed | Accepted | Deprecated | Superseded by ADR-XXX]

## Date

YYYY-MM-DD

## Context

What is the issue that we're seeing that is motivating this decision or change?

Include:
- Background information
- Problem statement
- Key requirements or constraints
- Technical context
- Business/product context if relevant

## Decision

What is the change that we're proposing and/or doing?

Be specific:
- What are we building/changing?
- What technology/approach are we using?
- What are the key technical decisions?
- Include code examples or diagrams if helpful

## Consequences

What becomes easier or more difficult to do because of this change?

### Positive

What are the benefits of this decision?
- Performance improvements
- Security benefits
- Maintainability gains
- Developer experience improvements
- etc.

### Negative

What are the downsides or limitations?
- Performance costs
- Complexity increases
- New dependencies
- Migration challenges
- etc.

### Trade-offs

What are we optimizing for vs. what are we accepting as trade-offs?
- Performance vs. simplicity
- Security vs. convenience
- Flexibility vs. constraints
- etc.

## Alternatives Considered

What other options did we consider?

For each alternative:
- **Alternative Name**
  - **Pros**: What were the benefits?
  - **Cons**: What were the drawbacks?
  - **Rejected because**: Why didn't we choose this?

## Implementation Notes

Optional section for technical details:
- Code examples
- Configuration examples
- Migration steps
- Deployment considerations
- Testing strategies

## References

- Links to related ADRs
- Links to code files implementing this decision
- External documentation
- Research papers or blog posts
- RFCs or specifications

---

## Guidelines for Writing ADRs

### When to Write an ADR

Write an ADR when making decisions that:
- Affect system architecture or design
- Have long-term consequences
- Involve trade-offs between multiple approaches
- Need to be communicated to team/stakeholders
- May need to be revisited or reversed later

### When NOT to Write an ADR

Don't write ADRs for:
- Routine implementation details
- Obvious decisions with no alternatives
- Temporary workarounds
- Personal preferences without technical impact

### ADR Principles

1. **Immutable**: ADRs are historical records. Don't edit after acceptance (except typos/clarifications)
2. **Supersedable**: If decision changes, create new ADR that supersedes old one
3. **Contextual**: Include enough context that future readers understand the decision
4. **Honest**: Document both pros and cons, don't oversell the decision
5. **Specific**: Include concrete examples, not just abstract descriptions

### Numbering

- Use sequential numbers: 001, 002, 003, etc.
- Don't renumber when superseding (new ADR gets new number)
- Reserve 000 for this template

### Lifecycle

```
Proposed → Accepted → [Deprecated → Superseded by ADR-XXX]
```

- **Proposed**: Decision under discussion
- **Accepted**: Decision made, implementation proceeding
- **Deprecated**: Decision still in effect but discouraged for new work
- **Superseded**: Decision replaced by newer ADR

### Review Process

1. Author creates ADR in `docs/adr/XXX-decision-name.md`
2. Team reviews (PR review, meeting, async discussion)
3. Address feedback, iterate on content
4. When consensus reached, update status to "Accepted"
5. Merge to main branch

### Maintenance

- Review ADRs during architecture reviews
- Update status if decisions change (don't edit the decision itself)
- Create superseding ADRs when decisions are revisited
- Keep ADRs in sync with code (reference ADRs in code comments)
