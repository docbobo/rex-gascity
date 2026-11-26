# 0006. The README release block changes only after a published release

- Status: Accepted
- Date: 2026-11-26

## Context

`README.md` tells a user which version to install: the `## Status:` heading, the
"is the current release" paragraph, and the `gc import add ... --version` pins.
PR #117 rewrote that block to `released, v2026.11.05` in a feature change, before
any `v2026.11.05` tag or GitHub release existed. The latest real release was
`v2026.11.04`. A user who followed the README ran an install that could not
complete (bead gcrex-ahvbi).

The same defect ran the other way after the release went out: the README still
said the tag was "not published yet" while the release was public. Both come from
one fault. The README release block changed on the schedule of a merge, not the
schedule of a release.

## Decision

We change the README release block only after the release is public, and only
through `pack/mark-released.sh v<version>`. A human runs it. It refuses unless
`gh release view` shows a published release that is neither a draft nor a
prerelease. It rewrites the release-state text, commits `docs: mark v<version>
released` and pushes `main`. It never tags, builds or publishes.

Feature changes do not touch the block. They also do not bump `pack/pack.toml` or
`ui/version`; those move in release prep only (`ui/AGENTS.md`).

## Consequences

- The README names the latest published release at all times, apart from the short
  window between publishing and running the script.
- Release prep leaves the README on the previous release. Publishing is no longer
  the last step: someone runs `mark-released.sh` after it.
- The script holds the exact release-state wording. A rewording of those README
  lines must change the script and `pack/tests/check-mark-released.sh` together.

## Alternatives rejected

- **Update the README in release prep, with the version bump.** This is what
  PR #117 did. The README then advertises a tag that does not exist until the
  draft is published, and a draft that is deleted or re-tagged leaves the README
  wrong.
- **Mark the README automatically when `release.yml` or `release.sh` publishes.**
  The requirement is a human-triggered step after the release is available to
  everyone. CI never publishes, and an automatic commit to `main` from the
  release path would also bypass review of that commit.
- **Gate the README with a CI check that compares it with the latest release.**
  It would fail every feature PR opened between a version bump and a publish, and
  it cannot tell "not yet published" from "forgot to mark".
