# GraphQL Developer Guide

### Modifying the GraphQL Schema

#### Fields

Add fields to .graphql files in the `schema/` folder. When you run
`make gqlgen`, these changes will be processed and a resolver will be generated.

You can also add models in `gqlgen.yml`. If a GraphQL object has a corresponding
model definition in `gqlgen.yml`, then a resolver will not be generated. Instead
you may have to edit the API models which are located in the `rest/model/`
folder.

#### Directives

Directives control access to certain mutations and queries. They are defined in
`schema/directives.graphql` but their corresponding functions are not generated
through `make gqlgen`. You will have to manually add or edit the directive
functions in `resolver.go`.

### Best Practices for GraphQL

#### Designing Mutations

When designing mutations, the input and payload should be objects. We often have
to add new fields, and it is much easier to add backwards compatible changes if
the existing fields are nested within an object.

In practice, this means you should prefer

```graphql
  abortTask(opts: AbortTaskInput!): AbortTaskPayload

  AbortTaskInput {
    taskId: String!
  }

  AbortTaskPayload {
    task: Task
  }
```

over

```graphql
abortTask(taskId: String!): Task
```

See the Apollo GraphQL
[blogpost](https://www.apollographql.com/blog/designing-graphql-mutations) from
which this was referenced.

Note that this guideline only applies to mutations, not queries.

#### Nullability

Nullability is controlled via the exclamation mark (`!`). If you put an
exclamation mark on a field, it means that the field cannot be null.

In general, you can reference this
[guide](https://yelp.github.io/graphql-guidelines/nullability.html#summary) for
nullability. Some callouts from this guide:

- Complex objects should be nullable due to the "bubbling up" effect.
- Lists should be non-nullable, and items contained within lists should be
  non-nullable.
- Booleans should be non-nullable. If you have a third state to represent,
  consider using an enum. You may also want to consider if the boolean field has
  the potential to evolve into an enum, such as in the example described
  [here](https://www.teamten.com/lawrence/programming/prefer-enums-over-booleans.html).

These principles apply generally, but you may encounter situations where you'll
want to deviate from these rules. Think carefully about marking fields as
non-nullable, because if we query for a non-nullable field and get null as a
response it will break parts of the application.

### Writing GraphQL tests

You can add tests to the `tests/` directory. The folder is structured as the
following:

```
.
├── ...
├── resolver/ # Folder representing a resolver, e.g. query
│ ├── field/ # Folder representing a field on a resolver, e.g. mainlineCommits
│ ├──── queries/ # Folder containing query files (.graphql)
│ ├──── data.json # Data for tests in this directory
│ └──── results.json # Results for tests in this directory
└── ...
```

The tests run via the test runner defined in `integration_atomic_test_util.go`.
If you see some behavior in your tests that can't be explained by what you've
added, it's a good idea to check the setup functions defined in this file.

Note: Tests for directives are located in `directive_test.go`.

#### Specifying User to Run GraphQL Test

The GraphQL schema restricts access to certain queries and mutations based on a user's permissions. In order to test that these restrictions are working properly, you may want to run a GraphQL test as a particular user.

There are three users available by default: 
- `admin_user`: A superuser with admin project and distro access.
- `privileged_user`: Not a superuser, but has admin project and distro access.
- `regular_user`: Only has basic project and distro access.

Note that you can also create other users in the corresponding `data.json` file if none of these users work for your test.

To run a test as a particular user, specify the `test_user_id` field in the corresponding `results.json` file. If this field is not included, it will default to `admin_user`.

### Running GraphQL tests

Before running any tests, ensure you have a creds.yml file set up. If you don't
have it, you can create one by running the following command:

```bash
bash scripts/setup-credentials.sh
```

To run all of the tests, you can use the following command:

```bash
SETTINGS_OVERRIDE=creds.yml make test-service-graphql
```

To run a specific test, you can use the following command:

```bash
SETTINGS_OVERRIDE=creds.yml make test-service-graphql RUN_TEST=TestAtomicGQLQueries/<TestName>
```
