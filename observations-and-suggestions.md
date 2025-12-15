### Billing Service — Observations and Suggestions

Group things according to domain
Move code that doesn't depend on our business logic into pkg folder e.g `internal/integrations`. 
Our pkg directory should contain functionality that can even be moved to different repo altogether.
Get rid of handlers folder and move api handlers code to their respective domains.
Domains in most cases have fuzzy boundaries.
