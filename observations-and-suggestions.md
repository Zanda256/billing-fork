### Billing Service — Observations and Suggestions

Implement a layered architecture with:
`app`  layer hosting logic for
  - Application startup
  - Application shutdown
  - Receive external input
  - Sending responses

`manager`/`business` layer
  - Code solving business problems
  - Should be reusable across apps
  - Folders in this layer include `web`, `data`, and `core` business logic. These can be further subdivided into 
    domains as illustrated this repo.
  - `web` provides reusable code related to our web API, e.g auth, metrics, response
  - `data` Data activities e.g migrations, CRUD, transaction
  - `api` Holds core business problems that get split into the domains we have

`pkg`/`foundation` layer
  - Many of the packages here can end up in a repo of their own.
  - Think of it as the standard lib of this project.
  - No configuration in this layer. Everything has to be passed in.

Data models
The structure in which data is represented in a database may differ from the way we want to present in to the user, 
or even differ from the structure we want to process it as.
This means we have to define suitable models at every stage. Models belong to a package, not the entire app. 
The application layer can have a different user model from the business layer, which then differs from the user model 
stored in cache and in postgres.

Key points
Group things according to domain e.g `payment` , `billing`, `notification`, `solana`
Move code that doesn't depend on our business logic into pkg folder e.g `internal/integrations`. 
Our pkg directory should contain functionality that can even be moved to different repo altogether.
Get rid of handlers folder and move api handlers code to their respective domains.
Domains in most cases have fuzzy boundaries.
