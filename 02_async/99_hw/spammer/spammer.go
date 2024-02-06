package main

import (
	"fmt"
	"sort"
	"sync"
)

func RunPipeline(cmds ...cmd) {
	in := make(chan interface{})
	out := make(chan interface{})
	WaitGr := &sync.WaitGroup{}
	for _, functionOfCmd := range cmds {
		WaitGr.Add(1)
		go func(in, out chan interface{}, functionOfCmd func(in, out chan interface{})) {
			defer func() {
				close(out)
				WaitGr.Done()
			}()
			functionOfCmd(in, out)
		}(in, out, functionOfCmd)
		in = out
		out = make(chan interface{})
	}
	WaitGr.Wait()
}

func SelectUsers(in, out chan interface{}) {
	uniqueIds := make(map[uint64]struct{})
	wg := &sync.WaitGroup{}
	mu := &sync.Mutex{}
	for currentEmail := range in {
		wg.Add(1)
		go func(currentEmail string) {
			defer wg.Done()
			currentUser := GetUser(currentEmail)
			func(currentUser User) {
				if checkIsIDUnique(uniqueIds, currentUser.ID, mu) {
					out <- currentUser
				}
			}(currentUser)
		}(currentEmail.(string))
	}
	wg.Wait()
}

func checkIsIDUnique(uniqueIds map[uint64]struct{}, newID uint64, mu *sync.Mutex) bool {
	mu.Lock()
	defer mu.Unlock()
	if _, exists := uniqueIds[newID]; !exists {
		uniqueIds[newID] = struct{}{}
		return true
	}
	return false
}

func SelectMessages(in, out chan interface{}) {
	var usersPull = make([]User, 0)
	wg := &sync.WaitGroup{}
	for currentUser := range in {
		usersPull = append(usersPull, currentUser.(User))
		if len(usersPull) >= GetMessagesMaxUsersBatch {
			selectMessageOfUsersBatch(usersPull[0:2], out, wg)
			usersPull = usersPull[2:]
		}
	}
	if len(usersPull) > 0 {
		selectMessageOfUsersBatch(usersPull, out, wg)
	}
	wg.Wait()
}

func selectMessageOfUsersBatch(usersBatch []User, out chan interface{}, wg *sync.WaitGroup) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		messageIdsForCurrentUserBatch, err := GetMessages(usersBatch...)
		if err == nil {
			for _, currentMessageID := range messageIdsForCurrentUserBatch {
				out <- currentMessageID
			}
		}
	}()
}

func CheckSpam(in, out chan interface{}) {
	antiBrutChanel := make(chan struct{}, HasSpamMaxAsyncRequests)
	wg := &sync.WaitGroup{}
	for currentMessage := range in {
		wg.Add(1)
		go func(antiBrutChanel chan struct{}, currentMessage MsgID) {
			antiBrutChanel <- struct{}{}
			defer func() {
				<-antiBrutChanel
				wg.Done()
			}()
			if spamInfo, err := HasSpam(currentMessage); err == nil {
				out <- MsgData{currentMessage, spamInfo}

			}
		}(antiBrutChanel, currentMessage.(MsgID))
	}
	wg.Wait()
}

func CombineResults(in, out chan interface{}) {
	var spamInfo MsgDataPull
	for currentResult := range in {
		spamInfo = append(spamInfo, currentResult.(MsgData))
	}
	sort.Sort(spamInfo)
	for _, currentSpamInfo := range spamInfo {
		currentSpamString := fmt.Sprintf("%v %d", currentSpamInfo.HasSpam, currentSpamInfo.ID)
		out <- currentSpamString
	}
}

type MsgDataPull []MsgData

func (msgD MsgDataPull) Len() int {
	return len(msgD)
}

func (msgD MsgDataPull) Less(i, j int) bool {
	if msgD[i].HasSpam != msgD[j].HasSpam {
		return msgD[i].HasSpam
	} else {
		return msgD[i].ID < msgD[j].ID
	}
}

func (msgD MsgDataPull) Swap(i, j int) {
	msgD[i], msgD[j] = msgD[j], msgD[i]
}
