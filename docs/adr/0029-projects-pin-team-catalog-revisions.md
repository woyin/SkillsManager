# Projects pin Team Catalog revisions

Projects record a resolved Team Catalog tag or commit as a Catalog Pin. Catalog changes are discovered as upgrade proposals by `sm plan` or `sm update`; only explicit confirmation updates the Pin and reconciles the project, while branch tracking remains an opt-in advanced mode.

Following a moving Catalog branch by default would allow Skills and required policies to alter Agent behavior or CI outcomes without a project-level review. Pins preserve reproducibility and permit staged rollout while retaining an intentional path for teams that value continuous tracking.
