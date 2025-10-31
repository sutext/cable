
namespace go api

struct JoinRequest {
	1: string uid
    2: list<string> channels
}
struct JoinResponse {
    1: i32 count
}

struct PublishRequest {
	1: string channel
    2: binary payload
}


service Service {
    JoinResponse join(1: JoinRequest req)
    JoinResponse leave(1: JoinRequest req)
    void publish(1: PublishRequest req)
}
