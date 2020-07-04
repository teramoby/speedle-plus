# mongodb support
mongodb could be used as a repository of speedle policies.
## 1. Prerequirements
mongodb instance 
### mongodb instance should be **replica sets** or **sharded cluster**
Note: since we use [change stream](https://docs.mongodb.com/manual/changeStreams/) to watch database changes, only replica sets and sharded cluster are supported.

For a quick test, we could [convert standalone instance to replica set](https://docs.mongodb.com/manual/tutorial/convert-standalone-to-replica-set/)

### mongodb cloud instance is also supported
[Get start with mongodb atlas](https://docs.atlas.mongodb.com/getting-started/)

## 2. Prepare config file
Here is a sample config file:
```
{
    "storeConfig": {
        "storeType": "mongodb",
        "storeProps": {
            "MongoURI": "mongodb+srv://userName:Password@cluster0.wfhda.mongodb.net/speedletest",
            "MongoDatabase": "speedletest"
        }
    },
    "enableWatch": true
}
```
Only MongoURI and MongoDatabase are required to be changed according to your env.

## 3. Start PMS and ADS service
### start PMS
```
speedle-pms --config-file pkg/svcs/pmsrest/config_mongodb.json
```
### start ADS
```
speedle-ads --config-file pkg/svcs/pmsrest/config_mongodb.json
```
