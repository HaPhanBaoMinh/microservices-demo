# Ad Service

The Ad service provides advertisement based on context keys. If no context keys are provided then it returns random ads.

## Building locally

The Ad service uses gradlew to compile/install/distribute. Gradle wrapper is already part of the source code. To build Ad Service, run:

```
./gradlew installDist
```

It will create executable script src/adservice/build/install/hipstershop/bin/AdService

### Upgrading gradle version

If you need to upgrade the version of gradle then run

```
./gradlew wrapper --gradle-version <new-version>
```

## Building docker image

From `src/adservice/`, run:

```
docker build ./
```

## Request json format

```json
{
  "contextKeys": ["key1", "key2"]
}
```

## Response json format

```json
{
  "ads": [
    {
      "redirect_url": "/product/66VCHSJNUP",
      "text": "Tank top for sale. 20% off."
    },
    {
      "redirect_url": "/product/2ZYFJ3GM2N",
      "text": "Hairdryer for sale. 50% off."
    }
  ]
}
```
