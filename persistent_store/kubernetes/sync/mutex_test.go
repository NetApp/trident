package sync

import (
	"reflect"
	"testing"
	"time"

	"github.com/pborman/uuid"
	"github.com/sirupsen/logrus"

	v1 "github.com/netapp/trident/persistent_store/kubernetes/apis/netapp/v1"
	"github.com/netapp/trident/persistent_store/kubernetes/client/clientset/versioned/fake"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func init() {
	// Print all information when tests fail
	logrus.SetLevel(logrus.DebugLevel)
}

// SetTimeToFixed points sets the time functions that Mutex uses to a
// fixed point, useful for testing
func SetTimeToFixedPoint(t time.Time) {
	Now = func() time.Time {
		return t
	}
}

// RestoreTimeToDefault resets the time functions that Mutex uses
func ReturnTimeToDefault() {
	Now = func() time.Time {
		return time.Now().UTC()
	}
}

func TestMutex_LockCreatesAndDeletesObjects(t *testing.T) {
	// Setup time
	SetTimeToFixedPoint(time.Now())
	defer ReturnTimeToDefault()

	// Setup objects
	client := fake.NewSimpleClientset().TridentV1().Mutexes()
	mutex := NewMutex(client)
	names := []string{"a", "b", "c"}

	// Create lock
	lock, err := mutex.Lock(names...)
	if err != nil {
		t.Fatal(err)
	}

	// Ensure objects created
	for _, name := range names {
		object, err := client.Get(name, metav1.GetOptions{})
		if err != nil {
			t.Fatal(err)
		}

		// Check object looks right
		expected := &v1.Mutex{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "trident.netapp.io/v1",
				Kind:       "Mutex",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Holder:  mutex.uuid,
			Expires: Now().UTC().Add(time.Second * 10).Unix(),
		}

		if !reflect.DeepEqual(expected, object) {
			t.Errorf("expected %v, got %v", expected, object)
		}
	}

	lock.Unlock()

	// Allow time for unlock in background
	time.Sleep(time.Millisecond)

	// Ensure objects deleted
	for _, name := range names {
		_, err := client.Get(name, metav1.GetOptions{})

		if !errors.IsNotFound(err) {
			t.Errorf("expected ErrIsNotFound, got %v", err)
		}
	}
}

func TestMutex_LockRemovesExpiredObjects(t *testing.T) {
	// Setup time
	SetTimeToFixedPoint(time.Now())
	defer ReturnTimeToDefault()

	// Setup objects
	client := fake.NewSimpleClientset().TridentV1().Mutexes()
	mutex := NewMutex(client)

	// Create expired mutex
	_, err := client.Create(&v1.Mutex{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "trident.netapp.io/v1",
			Kind:       "Mutex",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "expired",
		},
		Holder:  uuid.New(),
		Expires: Now().UTC().Add(-time.Second).Unix(),
	})

	if err != nil {
		t.Fatal(err)
	}

	// Lock over expired lock
	lock, err := mutex.Lock("expired")
	if err != nil {
		t.Fatal(err)
	}

	// Unlock to clean up goroutine (result not part of test)
	defer func() {
		lock.Unlock()
		time.Sleep(time.Millisecond)
	}()

	// Get object
	object, err := client.Get("expired", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// Check object looks right
	expected := &v1.Mutex{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "trident.netapp.io/v1",
			Kind:       "Mutex",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "expired",
		},
		Holder:  mutex.uuid,
		Expires: Now().UTC().Add(time.Second * 10).Unix(),
	}

	if !reflect.DeepEqual(expected, object) {
		t.Errorf("expected %v, got %v", expected, object)
	}
}

func TestMutex_LockWaitsForAcquire(t *testing.T) {
	// Setup time
	SetTimeToFixedPoint(time.Now())
	defer ReturnTimeToDefault()

	// Setup objects
	client := fake.NewSimpleClientset().TridentV1().Mutexes()
	mutex := NewMutex(client)
	secondLockAcquired := false

	// Lock a mutex
	lock, err := mutex.Lock("wait")
	if err != nil {
		t.Fatal(err)
	}

	// Start a goroutine to try for the same lock
	go func() {
		lock, err := mutex.Lock("wait")
		if err != nil {
			t.Fatal(err)
		}

		secondLockAcquired = true

		defer lock.Unlock()
	}()

	// Wait a couple of seconds to allow lock attempts
	time.Sleep(time.Second * 2)

	// Ensure second lock not acquired
	if secondLockAcquired {
		t.Fatal("second lock acquired")
	}

	// Unlock original lock
	lock.Unlock()

	// Wait a couple of seconds to allow lock attempts
	time.Sleep(time.Second * 2)

	// Ensure second lock now acquired
	if !secondLockAcquired {
		t.Fatal("second lock not acquired")
	}
}

func TestMutex_LockExpiryUpdates(t *testing.T) {
	// Setup time
	now := time.Now()
	SetTimeToFixedPoint(now)
	defer ReturnTimeToDefault()

	// Setup objects
	client := fake.NewSimpleClientset().TridentV1().Mutexes()
	mutex := NewMutex(client)

	// Lock a mutex
	lock, err := mutex.Lock("default")
	if err != nil {
		t.Fatal(err)
	}

	// Unlock to clean up goroutine (result not part of test)
	defer func() {
		lock.Unlock()
		time.Sleep(time.Millisecond)
	}()

	{
		// Get object
		object, err := client.Get("default", metav1.GetOptions{})
		if err != nil {
			t.Fatal(err)
		}

		// Check object looks right
		expected := &v1.Mutex{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "trident.netapp.io/v1",
				Kind:       "Mutex",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: "default",
			},
			Holder:  mutex.uuid,
			Expires: now.Add(time.Second * 10).Unix(),
		}

		if !reflect.DeepEqual(expected, object) {
			t.Errorf("original object incorrect: expected %v, got %v", expected, object)
		}
	}

	// Set time to point in future
	now = now.Add(time.Minute)
	SetTimeToFixedPoint(now)

	// Wait for update loop to perform update
	time.Sleep(time.Second * 5)

	{
		// Get object
		object, err := client.Get("default", metav1.GetOptions{})
		if err != nil {
			t.Fatal(err)
		}

		// Check object looks right
		expected := &v1.Mutex{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "trident.netapp.io/v1",
				Kind:       "Mutex",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: "default",
			},
			Holder:  mutex.uuid,
			Expires: now.Add(time.Second * 10).Unix(),
		}

		if !reflect.DeepEqual(expected, object) {
			t.Errorf("updated object incorrect: expected %v, got %v", expected, object)
		}
	}
}
