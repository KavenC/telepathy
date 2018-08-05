package telepathy

import (
	"context"
	"reflect"
	"testing"
)

func assertEqual(t *testing.T, a interface{}, b interface{}) {
	if reflect.DeepEqual(a, b) {
		t.Fatalf("%s != %s", a, b)
	}
}
func TestCreateUser(t *testing.T) {
	database = databaseList["mongo"]()

	testid := getReservedUserID("testUser")
	// Create test User object
	user := User{ID: testid, Privilege: 3, MsgID: make(map[string]string)}
	user.MsgID["msg1"] = "testid1"
	user.MsgID["msg2"] = "testid2"

	// Create User with database api
	err := database.createUser(&user)
	if err != nil {
		t.Fatal(err)
	}

	// Use database api to get User back and compare
	mongohmts := database.(*mongoDatabase)
	returnedUser := User{}
	collection := mongohmts.database.Collection(userCollection)
	result := collection.FindOne(context.Background(), map[string]string{"ID": testid})
	result.Decode(returnedUser)
	assertEqual(t, user, returnedUser)

	// Remove test user
	collection.DeleteOne(context.Background(), map[string]string{"ID": testid})
	result = collection.FindOne(context.Background(), map[string]string{"ID": testid})
	assertEqual(t, nil, result)
}

func TestFindUser(t *testing.T) {
	database = databaseList["mongo"]()

	testid := getReservedUserID("testUser")
	// Create test User object
	user := User{ID: testid, Privilege: 3, MsgID: make(map[string]string)}
	user.MsgID["msg1"] = "testid1"
	user.MsgID["msg2"] = "testid2"

	// Use database api to create User back and compare
	mongohmts := database.(*mongoDatabase)
	collection := mongohmts.database.Collection(userCollection)
	_, err := collection.InsertOne(context.Background(), user)
	if err != nil {
		t.Fatal(err)
	}

	// Test find user
	findUser := database.findUser(testid)
	assertEqual(t, user, findUser)

	// Remove test user
	collection.DeleteOne(context.Background(), map[string]string{"ID": testid})
	result := collection.FindOne(context.Background(), map[string]string{"ID": testid})
	assertEqual(t, nil, result)

	// Test user not found
	findUser = database.findUser(testid)
	assertEqual(t, findUser, nil)
}
